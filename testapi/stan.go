package main

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nats-io/stan.go"
)

func init() {
	clusterID := "stan"
	clientID := "dataflow-testapi"
	url := "nats"

	http.HandleFunc("/stan/pump-subject", func(w http.ResponseWriter, r *http.Request) {
		subjects := r.URL.Query()["subject"]
		if len(subjects) < 1 {
			w.WriteHeader(400)
			return
		}
		sleeps := r.URL.Query()["sleep"]
		if len(sleeps) < 1 {
			w.WriteHeader(400)
			return
		}
		duration, err := time.ParseDuration(sleeps[0])
		if err != nil {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		ns := r.URL.Query()["n"]
		if len(ns) < 1 {
			ns = []string{"-1"}
		}
		n, err := strconv.Atoi(ns[0])
		if err != nil {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(200)

		sc, err := stan.Connect(clusterID, clientID, stan.NatsURL(url))
		if err != nil {
			fmt.Printf("error: %v\n", err)
			w.WriteHeader(500)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		defer func() {
			// To wait for all ACKs are received
			time.Sleep(1 * time.Second)
			_ = sc.Close()
		}()

		start := time.Now()
		for i := 0; i < n || n < 0; i++ {
			select {
			case <-r.Context().Done():
				return
			default:
				x := fmt.Sprintf("%s-%d", FunnyAnimal(), i)
				_, err := sc.PublishAsync(subjects[0], []byte(x), func(ackedNuid string, err error) {
					if err != nil {
						fmt.Printf("Warning: error publishing msg id %s: %v\n", ackedNuid, err.Error())
					} else {
						fmt.Printf("Received ack for msg id %s\n", ackedNuid)
					}
				})

				if err != nil {
					_, _ = w.Write([]byte(fmt.Sprintf("Failed to publish message, error: %v\n", err.Error())))
					return
				}
				_, _ = fmt.Fprintf(w, "sent %q (%.0f TPS) to %q\n", x, (1+float64(i))/time.Since(start).Seconds(), subjects[0])
				time.Sleep(duration)
			}
		}
	})
}
