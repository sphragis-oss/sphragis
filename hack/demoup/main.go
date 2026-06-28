// SPDX-License-Identifier: Apache-2.0

// Command demoup is a plain-HTTP stand-in LLM for the wireshark redaction demo.
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		fmt.Printf("\n[LLM RECEIVED on %s]\n%s\n", r.URL.Path, string(b))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"demo","content":[{"type":"text","text":"ok"}]}`))
	})
	fmt.Println("demo LLM upstream listening on 127.0.0.1:8901")
	log.Fatal(http.ListenAndServe("127.0.0.1:8901", nil))
}
