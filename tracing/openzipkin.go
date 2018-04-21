package tracing

import (
	"log"

	"github.com/openzipkin/zipkin-go"
	zipkinhttp "github.com/openzipkin/zipkin-go/middleware/http"
	zipkinreporter "github.com/openzipkin/zipkin-go/reporter"
	httpreporter "github.com/openzipkin/zipkin-go/reporter/http"
)

var reporter zipkinreporter.Reporter
var tracer *zipkin.Tracer

func Initialize(c Config) {
	if !c.Enabled || c.EndpointURL == "" {
		return
	}

	// Create a HTTP span reporter
	reporter = httpreporter.NewReporter(c.EndpointURL)

	// Create our local service endpoint
	endpoint, err := zipkin.NewEndpoint(c.ServiceName, c.ServiceHostPort)
	if err != nil {
		log.Fatalf("unable to create local endpoint: %+v", err)
	}

	// Create our tracer
	tracer, err = zipkin.NewTracer(reporter, zipkin.WithLocalEndpoint(endpoint))
	if err != nil {
		log.Fatalf("unable to create tracer: %+v", err)
	}
}

func GetTracer() *zipkin.Tracer {
	return tracer
}

func GetHttpClient() *zipkinhttp.Client {
	if tracer == nil {
		return nil
	}

	client, err := zipkinhttp.NewClient(tracer, zipkinhttp.ClientTrace(true))
	if err != nil {
		log.Fatalf("unable to create client: %+v\n", err)
	}

	return client
}

func Shutdown() {
	if reporter != nil {
		reporter.Close()
	}
}
