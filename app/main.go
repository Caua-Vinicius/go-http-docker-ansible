package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Response struct {
	Nome    string `json:"nome"`
	Horario string `json:"horario"`
}

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total de requisições recebidas",
		},
		[]string{"method", "endpoint", "status"},
	)

	serviceUp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "service_up",
			Help: "Disponibilidade do serviço (1 = up, 0 = down)",
		},
	)
)

func init() {
	prometheus.MustRegister(requestsTotal)
	prometheus.MustRegister(serviceUp)
	serviceUp.Set(1)
}

func korpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		requestsTotal.WithLabelValues(r.Method, "/projeto-korp", "405").Inc()
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := Response{
		Nome:    "Projeto Korp",
		Horario: time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
	requestsTotal.WithLabelValues(r.Method, "/projeto-korp", "200").Inc()
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/projeto-korp", korpHandler)
	mux.Handle("/metrics", promhttp.Handler())

	log.Println("http-server-projeto-korp is running on port 8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("error starting server: %v", err)
	}
}
