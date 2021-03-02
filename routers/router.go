package routers

import (
	"github.com/go-chi/chi"
	"github.com/kiali/kiali/handlers"
	"github.com/kiali/kiali/kubernetes"
	"github.com/kiali/kiali/log"
	"github.com/opentracing/opentracing-go"
	httpSwagger "github.com/swaggo/http-swagger"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
	"github.com/uber/jaeger-lib/metrics"
	"net/http"
)

// Route describes a single route
type Route struct {
	Name          string
	Method        string
	Pattern       string
	HandlerFunc   http.HandlerFunc
	Authenticated bool
}

// Routes holds an array of Route. A note on swagger documentation. The path variables and query parameters
// are defined in ../doc.go.  YOu need to manually associate params and routes.
type Routes struct {
	Routes []Route
}

// NewRoutes creates and returns all the API routes
func NewRoutes(graphController *handlers.GraphController) (r *Routes) {
	r = new(Routes)
	r.Routes = []Route{
		{
			"Healthz",
			http.MethodGet,
			"/healthz",
			handlers.Healthz,
			false,
		},
		{
			"Graph-Namespace",
			http.MethodPost,
			"/graph/namespace/{namespace}/duration/{duration}/deadEdges/{deadEdges}/passThrough/{passThrough}/graphType/{graphType}",
			graphController.GetNamespacesController,
			false,
		},
		{
			"Graph-test",
			http.MethodGet,
			"/graph",
			handlers.GraphNamespaces,
			false,
		},
		{
			"Graph-Node",
			http.MethodPost,
			"/graph/namespace/{namespace}/service/{service}/duration/{duration}/deadEdges/{deadEdges}/passThrough/{passThrough}",
			graphController.GetNodeController,
			false,
		},
	}
	return
}

func NewRouter(prometheusUrl, context string) (*chi.Mux, error) {
	r := chi.NewRouter()
	configClient, err := kubernetes.ConfigClient()
	if err != nil {
		return nil, err
	}
	clientSet, err := kubernetes.GetDefaultK8sClientSet()
	if err != nil {
		return nil, err
	}
	graphController := handlers.NewGraphController(configClient, clientSet, prometheusUrl, context)
	apiRoutes := NewRoutes(graphController)
	// swagger api html ---> http://localhost:8000/swagger/index.html#/
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("http://localhost:8080/swagger/doc.json"), //The url pointing to API definition"
	))
	for _, api := range apiRoutes.Routes {
		r.MethodFunc(api.Method, api.Pattern, api.HandlerFunc)
	}
	return r, nil
}

func InitOpentracing(jaegerUrl string) error {
	cfg := jaegercfg.Configuration{
		ServiceName: "solar-graph.service-mesh",
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegercfg.ReporterConfig{
			LogSpans:           true,
			LocalAgentHostPort: jaegerUrl, // 替换host
		},
	}
	jLogger := jaegerlog.StdLogger
	jMetricsFactory := metrics.NullFactory
	tracer, _, err := cfg.NewTracer(
		jaegercfg.Logger(jLogger),
		jaegercfg.Metrics(jMetricsFactory),
	)
	opentracing.SetGlobalTracer(tracer)
	if err != nil {
		log.Errorf("Could not initialize jaeger tracer: %s", err.Error())
		return err
	}
	return err
}
