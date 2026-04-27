package main

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/alecthomas/kong"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/takuo/epgstation-exporter/internal/collector"
)

var cli struct {
	APIURL        string `help:"EPGStation API URL" default:"http://localhost:8888/api" env:"EPGSTATION_API_URL" name:"api-url"`
	Port          int    `help:"Listen port" default:"9888" env:"EPGSTATION_EXPORTER_PORT" name:"port"`
	MetricsPath   string `help:"Metrics path" default:"/metrics" env:"EPGSTATION_METRICS_PATH" name:"metrics-path"`
	EnableStorage bool   `help:"Enable storage metrics" default:"true" env:"EPGSTATION_ENABLE_STORAGE" negatable:"" name:"enable-storage"`
	EnableStreams  bool   `help:"Enable streams metrics" default:"true" env:"EPGSTATION_ENABLE_STREAMS" negatable:"" name:"enable-streams"`
}

func main() {
	kong.Parse(&cli,
		kong.Name("epgstation-exporter"),
		kong.Description("Prometheus exporter for EPGStation"),
	)

	c, err := collector.New(cli.APIURL, cli.EnableStorage, cli.EnableStreams)
	if err != nil {
		slog.Error("failed to create collector", "err", err)
		panic(err)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mux := http.NewServeMux()
	mux.Handle(cli.MetricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `<html>
<head><title>EPGStation Exporter</title></head>
<body>
<h1>EPGStation Exporter</h1>
<p><a href="%s">Metrics</a></p>
</body>
</html>`, cli.MetricsPath)
	})

	addr := fmt.Sprintf(":%d", cli.Port)
	slog.Info("starting epgstation-exporter", "addr", addr, "metricsPath", cli.MetricsPath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server error", "err", err)
		panic(err)
	}
}
