package main

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"

	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
)

func do(ctx context.Context, fn func(msg []byte) ([][]byte, error)) error {
	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	http.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		in, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Error(err, "failed to marshal message")
			w.WriteHeader(500)
			return
		}
		msgs, err := fn(in)
		if err != nil {
			log.Error(err, "failed execute")
			w.WriteHeader(500)
			return
		}
		for _, out := range msgs {
			resp, err := http.Post("http://localhost:3569/messages", "application/json", bytes.NewBuffer(out))
			if err != nil {
				log.Error(err, "failed to post message")
				w.WriteHeader(500)
				return
			}
			if resp.StatusCode != 200 {
				log.Error(err, "failed to post message", resp.Status)
				w.WriteHeader(500)
				return
			}
			log.WithValues("in", short(in), "out", short(out)).Info("do")
		}
		w.WriteHeader(200)
	})

	go func() {
		defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)
		if err := http.ListenAndServe(":8080", nil); err != nil {
			panic(err)
		}
	}()

	<-ctx.Done()

	return nil
}
