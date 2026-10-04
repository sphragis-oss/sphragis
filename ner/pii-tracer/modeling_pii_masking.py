"""Detect and mask PII with pplx-pii-masking.

Loaded through `AutoModel`, which pulls this file in as remote code:

    from transformers import AutoModel

    model = AutoModel.from_pretrained(
        "perplexity-ai/pplx-pii-masking", trust_remote_code=True
    )
    spans, sensitivity = model.predict("Hi, I'm Daniel Whitfield")
    print(model.mask("Hi, I'm Daniel Whitfield"))

The encoder is the standard Transformers `Qwen3Model` run bidirectionally via
`config.is_causal = False` (requires `transformers>=5.2`, which passes
`is_causal` through to the attention backend), the fine-tuned weights come
from this repo, and the constrained BIOES Viterbi decoder is inlined below.
"""
from __future__ import annotations

from dataclasses import dataclass

import torch
from torch import nn
from transformers import (
    AutoTokenizer,
    PretrainedConfig,
    PreTrainedModel,
    Qwen3Config,
    Qwen3Model,
)
from transformers.utils import ModelOutput
from transformers.utils.versions import require_version

require_version(
    "transformers>=5.2.0",
    "pplx-pii-masking requires Transformers 5.2 or newer for bidirectional Qwen3 attention.",
)

# from_pretrained kwargs that also apply when loading the tokenizer next to the weights
HUB_KWARGS = ("cache_dir", "force_download", "local_files_only", "proxies",
              "revision", "subfolder", "token", "trust_remote_code")

PII_TYPES = [
    "private_person",
    "private_email",
    "private_phone",
    "private_address",
    "private_url",
    "private_date",
    "account_number",
    "secret",
    "other_pii",
]
# BIOES label list: O + 9 types x {B, I, E, S} = 37 (matches the checkpoint)
BIOES_LABELS = ["O"] + [f"{tag}-{t}" for t in PII_TYPES for tag in "BIES"]


# ---------------------------------------------------------------------------
# Constrained BIOES Viterbi decoder
# ---------------------------------------------------------------------------

@dataclass
class PredictedSpan:
    start: int   # character offset in document text
    end: int
    label: str   # e.g. "private_person"
    score: float


def strip_span_whitespace(text: str, spans: list[PredictedSpan]) -> list[PredictedSpan]:
    """Trim leading/trailing whitespace from predicted character spans.

    Tokenizers fuse the space before a word into the token (e.g. '_john'),
    so the B-token's char_start is one position before the actual PII value.
    """
    out: list[PredictedSpan] = []
    for s in spans:
        new_start, new_end = s.start, s.end
        while new_start < new_end and text[new_start] in (" ", "\t", "\n"):
            new_start += 1
        while new_end > new_start and text[new_end - 1] in (" ", "\t", "\n"):
            new_end -= 1
        if new_start < new_end:
            out.append(PredictedSpan(new_start, new_end, s.label, s.score))
    return out


class ViterbiDecoder(nn.Module):
    """Constrained BIOES Viterbi decoder with transition bias scalars.

    The bias scalars are added to all ->B and E-> transitions, allowing
    precision/recall trade-offs without retraining. They and the legal
    transition mask are buffers, so they come from the checkpoint.
    """

    def __init__(self, labels: list[str], b_bias: float = 0.0, e_bias: float = 0.0):
        super().__init__()
        self.labels = labels
        self.num_labels = len(labels)
        self.label2id = {label: idx for idx, label in enumerate(labels)}
        self.id2label = {idx: label for idx, label in enumerate(labels)}
        self.pii_types = [label[2:] for label in labels if label.startswith("S-")]
        self.register_buffer("b_bias", torch.tensor([float(b_bias)]))
        self.register_buffer("e_bias", torch.tensor([float(e_bias)]))
        self.register_buffer("transition_mask", self._build_transition_mask())

    def _build_transition_mask(self) -> torch.Tensor:
        """[num_labels, num_labels] bool mask of the legal tag transitions."""
        n, l2i = self.num_labels, self.label2id
        mask = torch.zeros(n, n, dtype=torch.bool)
        end_states = {l2i["O"]}    # states a span can end on (O, E-*, S-*)
        begin_states = {l2i["O"]}  # states valid after a boundary (O, B-*, S-*)
        for pii_type in self.pii_types:
            b, i = l2i[f"B-{pii_type}"], l2i[f"I-{pii_type}"]
            e, s = l2i[f"E-{pii_type}"], l2i[f"S-{pii_type}"]
            end_states |= {e, s}
            begin_states |= {b, s}
            mask[b, i] = mask[b, e] = True   # B -> I/E of same type
            mask[i, i] = mask[i, e] = True   # I -> I/E of same type
        for from_state in end_states:
            for to_state in begin_states:
                mask[from_state, to_state] = True
        return mask

    def _build_transition_scores(self) -> torch.Tensor:
        """[num_labels, num_labels] float transition score matrix."""
        n, l2i = self.num_labels, self.label2id
        b_bias, e_bias = float(self.b_bias), float(self.e_bias)
        scores = torch.full((n, n), float("-inf"))
        scores[self.transition_mask.bool().cpu()] = 0.0  # decoding runs on cpu
        for pii_type in self.pii_types:
            b, e = l2i[f"B-{pii_type}"], l2i[f"E-{pii_type}"]
            for from_s in range(n):
                if scores[from_s, b] > float("-inf"):
                    scores[from_s, b] += b_bias   # entering B states
            for to_s in range(n):
                if scores[e, to_s] > float("-inf"):
                    scores[e, to_s] += e_bias     # leaving E states
        return scores

    @torch.no_grad()
    def decode(
        self,
        logits: torch.Tensor,
        offset_mapping: list[tuple[int, int]],
        text: str | None = None,
    ) -> list[PredictedSpan]:
        """Decode a single sequence.

        Args:
            logits: [T, num_labels] float tensor
            offset_mapping: list of (char_start, char_end) per token
            text: source text; when provided, leading/trailing whitespace is
                stripped from predicted span boundaries.
        """
        T, C = logits.shape
        assert C == self.num_labels

        trans = self._build_transition_scores()  # [C, C]

        viterbi_scores = torch.full((T, C), float("-inf"))
        backpointers = torch.zeros((T, C), dtype=torch.long)

        # t=0: only O / B-* / S-* are valid start states
        start_mask = torch.full((C,), float("-inf"))
        for label, idx in self.label2id.items():
            if label == "O" or label.startswith("B-") or label.startswith("S-"):
                start_mask[idx] = 0.0
        viterbi_scores[0] = logits[0] + start_mask

        for t in range(1, T):
            # [C, 1] + [C, C] -> [C, C]; dim-0 = prev, dim-1 = next
            scores_t = viterbi_scores[t - 1].unsqueeze(1) + trans
            best_prev, best_idx = scores_t.max(dim=0)
            viterbi_scores[t] = logits[t] + best_prev
            backpointers[t] = best_idx

        # End constraint: only O / E-* / S-* valid at end
        end_mask = torch.full((C,), float("-inf"))
        for label, idx in self.label2id.items():
            if label == "O" or label.startswith("E-") or label.startswith("S-"):
                end_mask[idx] = 0.0
        best_last = int((viterbi_scores[T - 1] + end_mask).argmax().item())

        path = [best_last]
        for t in range(T - 1, 0, -1):
            path.append(int(backpointers[t, path[-1]].item()))
        path.reverse()

        spans = self._path_to_spans(path, logits, offset_mapping)
        if text is not None:
            spans = strip_span_whitespace(text, spans)
        return spans

    def _path_to_spans(
        self,
        path: list[int],
        logits: torch.Tensor,
        offset_mapping: list[tuple[int, int]],
    ) -> list[PredictedSpan]:
        spans: list[PredictedSpan] = []
        T = len(path)
        t = 0
        while t < T:
            label = self.id2label[path[t]]
            char_start, char_end = offset_mapping[t]

            if label.startswith("S-"):
                pii_type = label[2:]
                score = float(logits[t, path[t]].sigmoid().item())
                if char_start < char_end:
                    spans.append(PredictedSpan(char_start, char_end, pii_type, score))
                t += 1

            elif label.startswith("B-"):
                pii_type = label[2:]
                span_start, span_end = char_start, char_end
                tok_scores = [float(logits[t, path[t]].item())]
                t += 1
                while t < T:
                    inner = self.id2label[path[t]]
                    if inner == f"I-{pii_type}":
                        _, span_end = offset_mapping[t]
                        tok_scores.append(float(logits[t, path[t]].item()))
                        t += 1
                    elif inner == f"E-{pii_type}":
                        _, span_end = offset_mapping[t]
                        tok_scores.append(float(logits[t, path[t]].item()))
                        t += 1
                        break
                    else:
                        break
                if span_start < span_end:
                    score = float(torch.tensor(tok_scores).mean().sigmoid().item())
                    spans.append(PredictedSpan(span_start, span_end, pii_type, score))

            else:
                t += 1

        return spans


# ---------------------------------------------------------------------------
# Model
# ---------------------------------------------------------------------------

class PiiMaskingConfig(PretrainedConfig):
    """Config for `PiiMaskingModel`.

    `backbone` stays a plain dict; `PiiMaskingModel` turns it into a
    `Qwen3Config`.
    """

    model_type = "pii_masking"

    def __init__(
        self,
        backbone: dict | None = None,
        hidden_size: int = 1024,
        num_token_labels: int = len(BIOES_LABELS),
        max_seq_len: int = 4096,
        viterbi_b_bias: float = 0.0,
        viterbi_e_bias: float = 0.0,
        **kwargs,
    ):
        self.backbone = backbone or {}
        self.hidden_size = hidden_size
        self.num_token_labels = num_token_labels
        self.max_seq_len = max_seq_len
        self.viterbi_b_bias = viterbi_b_bias
        self.viterbi_e_bias = viterbi_e_bias
        kwargs.setdefault("id2label", dict(enumerate(BIOES_LABELS)))
        kwargs.setdefault(
            "label2id", {label: i for i, label in enumerate(BIOES_LABELS)}
        )
        super().__init__(**kwargs)


@dataclass
class PiiMaskingOutput(ModelOutput):
    """[B, T, 37] tag logits, [B] sensitivity logits, [B, T, H] encoder output."""

    logits: torch.FloatTensor | None = None
    sensitivity_logits: torch.FloatTensor | None = None
    last_hidden_state: torch.FloatTensor | None = None


class PiiMaskingModel(PreTrainedModel):
    """Bidirectional Qwen3 encoder with PII token and sensitivity heads."""

    config_class = PiiMaskingConfig
    _supports_sdpa = True
    _supports_flash_attn = True
    # The heads and the decoder biases are fp32 in the checkpoint while the
    # encoder is bf16. Keep them fp32 whichever dtype the model is loaded in.
    _keep_in_fp32_modules_strict = [
        "token_cls_head", "sensitivity_head", "viterbi.b_bias", "viterbi.e_bias",
    ]

    def __init__(self, config: PiiMaskingConfig):
        super().__init__(config)
        backbone_config = Qwen3Config.from_dict(config.backbone)
        # This model is a bidirectional encoder regardless of what the dict
        # says; transformers >= 5.2 passes is_causal through to the backend.
        backbone_config.is_causal = False
        backbone_config._attn_implementation = config._attn_implementation
        self.backbone = Qwen3Model(backbone_config)
        self.token_cls_head = nn.Linear(config.hidden_size, config.num_token_labels)
        self.sensitivity_head = nn.Linear(config.hidden_size, 1)
        self.viterbi = ViterbiDecoder(
            BIOES_LABELS, config.viterbi_b_bias, config.viterbi_e_bias
        )
        self._tokenizer = None
        self._tokenizer_kwargs: dict = {}
        self.post_init()

    @classmethod
    def from_pretrained(cls, pretrained_model_name_or_path, *model_args, **kwargs):
        loaded = super().from_pretrained(
            pretrained_model_name_or_path, *model_args, **kwargs
        )
        model = loaded[0] if isinstance(loaded, tuple) else loaded
        # Same revision / token / cache as the weights, so `predict` on a model
        # loaded from a branch or a private repo finds the matching tokenizer.
        model._tokenizer_kwargs = {k: kwargs[k] for k in HUB_KWARGS if k in kwargs}
        # The wrapper itself has already been authorized as remote code. Its
        # config is custom too, so let AutoTokenizer resolve that config without
        # showing a second trust prompt during the first predict() call.
        model._tokenizer_kwargs.setdefault("trust_remote_code", True)
        return loaded

    def save_pretrained(self, save_directory, *args, **kwargs):
        super().save_pretrained(save_directory, *args, **kwargs)
        # The saved directory must work with `predict`, which needs the tokenizer.
        if self._tokenizer is not None or self.config._name_or_path:
            self.tokenizer.save_pretrained(save_directory)

    @property
    def tokenizer(self):
        """Tokenizer used by `predict`, loaded from the checkpoint on first use."""
        if self._tokenizer is None:
            self._tokenizer = AutoTokenizer.from_pretrained(
                self.config._name_or_path, **self._tokenizer_kwargs
            )
        return self._tokenizer

    @tokenizer.setter
    def tokenizer(self, value):
        self._tokenizer = value

    def forward(
        self,
        input_ids: torch.LongTensor | None = None,
        attention_mask: torch.Tensor | None = None,
        **kwargs,
    ) -> PiiMaskingOutput:
        last_hidden_state = self.backbone(
            input_ids=input_ids, attention_mask=attention_mask, **kwargs
        ).last_hidden_state                                        # [B, T, 1024]
        h = last_hidden_state.to(self.token_cls_head.weight.dtype)  # heads are fp32

        if attention_mask is None:
            pooled = h.mean(dim=1)
        else:
            mask = attention_mask.to(h.dtype).unsqueeze(-1)
            pooled = (h * mask).sum(dim=1) / mask.sum(dim=1).clamp(min=1.0)

        return PiiMaskingOutput(
            logits=self.token_cls_head(h),
            sensitivity_logits=self.sensitivity_head(pooled).squeeze(-1),
            last_hidden_state=last_hidden_state,
        )

    @torch.no_grad()
    def predict(
        self,
        text: str,
        tokenizer=None,
        max_length: int | None = None,
    ) -> tuple[list[PredictedSpan], float]:
        """Predicted PII spans, and the document sensitivity in [0, 1].

        Text past `max_length` tokens is truncated, so spans there are not
        reported. An empty document has no spans and sensitivity 0.0.
        """
        tokenizer = tokenizer if tokenizer is not None else self.tokenizer
        enc = tokenizer(text, return_offsets_mapping=True, return_tensors="pt",
                        truncation=True,
                        max_length=max_length or self.config.max_seq_len)
        if enc["input_ids"].shape[1] == 0:  # empty document, nothing to encode
            return [], 0.0
        out = self(
            input_ids=enc["input_ids"].to(self.device),
            attention_mask=enc["attention_mask"].to(self.device),
        )
        offsets = [tuple(o) for o in enc["offset_mapping"][0].tolist()]
        spans = self.viterbi.decode(out.logits[0].float().cpu(), offsets, text=text)
        return spans, float(out.sensitivity_logits.float().sigmoid())

    def mask(self, text: str, placeholder: str = "[{label}]", **kwargs) -> str:
        """The text with every predicted span replaced by e.g. [PRIVATE_EMAIL]."""
        spans, _ = self.predict(text, **kwargs)
        for s in sorted(spans, key=lambda s: -s.start):
            filled = placeholder.format(label=s.label.upper())
            text = text[:s.start] + filled + text[s.end:]
        return text
