package api

import (
	"fmt"
	"github.com/opentracing/opentracing-go"
	"net/http"

	"github.com/kiali/kiali/business"
	"github.com/kiali/kiali/graph"
	"github.com/kiali/kiali/graph/config/cytoscape"
	"github.com/kiali/kiali/graph/telemetry/istio"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/prometheus"
	"github.com/kiali/kiali/prometheus/internalmetrics"
)

type GraphApi struct {
	business *business.Layer
	options  graph.Options
	option   graph.Option
}

type GraphApiInterface interface {
	RegistryHandle(span opentracing.Span, loads map[string]interface{}) (edges []*cytoscape.EdgeWrapper, err error)
	NodeRegistryHandle(span opentracing.Span, loads map[string]interface{}) (edges []*cytoscape.EdgeWrapper, err error)
}

func NewGraphApi(option graph.Option, span opentracing.Span) (*GraphApi, error) {
	graphApi := opentracing.StartSpan("graph", opentracing.ChildOf(span.Context()))
	graphApi.SetTag("namespaces", option.Namespaces)
	graphApi.LogKV("new graph options", fmt.Sprintf("prometheus address:%s", option.Prometheus))
	o, err := option.NewGraphOptions(option.Config, option.Prometheus)
	if err != nil {
		return nil, err
	}
	businessNoAuth, err := business.GetNoAuth(option.Config, option.Prometheus, graphApi)
	if err != nil {
		return nil, err
	}
	return &GraphApi{business: businessNoAuth, options: o, option: option}, nil
}

func (g *GraphApi) RegistryHandle(span opentracing.Span, loads map[string]interface{}) (edges []*cytoscape.EdgeWrapper, err error) {
	graphNamespacesCluster(g.business, g.options, span, loads)
	if g.options.PassThrough {
		return passEdges(g.business, g.options, span)

	}
	return edges, err
}

func (g *GraphApi) NodeRegistryHandle(span opentracing.Span, loads map[string]interface{}) (edges []*cytoscape.EdgeWrapper, err error) {
	graphNodeCluster(g.business, g.options, span, loads)
	if g.options.PassThrough {
		return passNodeEdges(g.business, g.options, span)

	}
	return edges, err
}

// graphNamespacesCluster 单个集群的namespaces 级别的流量视图
func graphNamespacesCluster(business *business.Layer, o graph.Options, span opentracing.Span, loads map[string]interface{}) {
	graphNamespacesSpan := opentracing.StartSpan("get graph", opentracing.FollowsFrom(span.Context()))
	graphNamespacesSpan.LogKV("func GraphNamespaces start", "")
	_, payload, err := GraphNamespaces(business, o, graphNamespacesSpan)
	graphNamespacesSpan.Finish()
	if err != nil {
		return
	}
	loads[o.Context] = payload
}

//graphNodeCluster  单个集群的 某个节点 级别的流量视图
func graphNodeCluster(business *business.Layer, o graph.Options, span opentracing.Span, loads map[string]interface{}) {
	graphNamespacesSpan := opentracing.StartSpan("get graph", opentracing.FollowsFrom(span.Context()))
	graphNamespacesSpan.LogKV("func GraphNamespaces start", "")
	_, payload := GraphNode(business, o)
	graphNamespacesSpan.Finish()
	/*	if err != nil {
		return
	}*/
	loads[o.Context] = payload
}

//passEdges 获取当前集群跨集群的线
func passEdges(businessNoAuth *business.Layer, o graph.Options, optionSpan opentracing.Span) (edges []*cytoscape.EdgeWrapper, err error) {
	passSpan := opentracing.StartSpan("pass Through", opentracing.ChildOf(optionSpan.Context()))
	passSpan.LogKV("func GraphNamespaces start", "")
	return passThroughEdges(o, businessNoAuth)
}

//passEdges 获取当前集群跨集群的线
func passNodeEdges(businessNoAuth *business.Layer, o graph.Options, optionSpan opentracing.Span) (edges []*cytoscape.EdgeWrapper, err error) {
	passSpan := opentracing.StartSpan("pass Through", opentracing.ChildOf(optionSpan.Context()))
	passSpan.LogKV("func GraphNamespaces start", "")
	return passThroughEdgesNode(o, businessNoAuth)
}

// GraphNamespaces generates a namespaces graph using the provided options
func GraphNamespaces(business *business.Layer, o graph.Options, span opentracing.Span) (code int, config interface{}, err error) {
	// time how long it takes to generate this graph
	promtimer := internalmetrics.GetGraphGenerationTimePrometheusTimer(o.GetGraphKind(), o.TelemetryOptions.GraphType, o.InjectServiceNodes)
	defer promtimer.ObserveDuration()

	switch o.TelemetryVendor {
	case graph.VendorIstio:
		span.LogKV("TelemetryVendor", graph.VendorIstio)
		prom, err := prometheus.NewClientNoAuth(business.PromAddress)
		if err != nil {
			return 0, nil, err
		}
		code, config = graphNamespacesIstio(business, prom, o, span)
	default:
		span.LogKV("TelemetryVendor", graph.VendorIstio)
		graph.Error(fmt.Sprintf("TelemetryVendor [%s] not supported", o.TelemetryVendor))
	}

	// update metrics
	internalmetrics.SetGraphNodes(o.GetGraphKind(), o.TelemetryOptions.GraphType, o.InjectServiceNodes, 0)

	return code, config, nil
}

// GraphNode generates a node graph using the provided options
// Get cross-cluster traffic lines
func GraphNode(business *business.Layer, o graph.Options) (code int, config interface{}) {
	if len(o.Namespaces) != 1 {
		graph.Error(fmt.Sprintf("Node graph does not support the 'namespaces' query parameter or the 'all' namespace"))
	}

	// time how long it takes to generate this graph
	promtimer := internalmetrics.GetGraphGenerationTimePrometheusTimer(o.GetGraphKind(), o.TelemetryOptions.GraphType, o.InjectServiceNodes)
	defer promtimer.ObserveDuration()

	switch o.TelemetryVendor {
	case graph.VendorIstio:
		prom, err := prometheus.NewClientNoAuth(business.PromAddress)
		graph.CheckError(err)
		//
		code, config = graphNodeIstio(business, prom, o)
	default:
		graph.Error(fmt.Sprintf("TelemetryVendor [%s] not supported", o.TelemetryVendor))
	}
	// update metrics
	internalmetrics.SetGraphNodes(o.GetGraphKind(), o.TelemetryOptions.GraphType, o.InjectServiceNodes, 0)

	return code, config
}

//passThrough 线
func passThroughEdges(o graph.Options, business *business.Layer) (edge []*cytoscape.EdgeWrapper, err error) {
	prom, err := prometheus.NewClientNoAuth(business.PromAddress)
	if err != nil {
		return
	}
	globalInfo := graph.NewAppenderGlobalInfo()
	globalInfo.Context = o.Context
	globalInfo.Business = business
	globalInfo.PromClient = prom
	edgs, err := istio.AddMultiClusterEdge(o.TelemetryOptions, globalInfo, o.Clusters, o.Context)
	if err != nil {
		log.Debugf("%v", edgs)
		return
	}
	edge = cytoscape.NewMultiClusterEdge(edgs, o)
	return
}

//passThroughEdgesNode
func passThroughEdgesNode(o graph.Options, business *business.Layer) (edge []*cytoscape.EdgeWrapper, err error) {
	prom, err := prometheus.NewClientNoAuth(business.PromAddress)
	if err != nil {
		return
	}
	globalInfo := graph.NewAppenderGlobalInfo()
	globalInfo.Context = o.Context
	globalInfo.Business = business
	globalInfo.PromClient = prom
	edgs, err := istio.NodeMultiClusterEdge(o, globalInfo, o.Clusters, o.Context)
	if err != nil {
		log.Debugf("%v", edgs)
		return
	}
	edge = cytoscape.NewMultiClusterEdge(edgs, o)
	return
}

// graphNamespacesIstio provides a test hook that accepts mock clients
func graphNamespacesIstio(business *business.Layer, prom *prometheus.Client, o graph.Options, span opentracing.Span) (code int, config cytoscape.Config) {
	// Create a 'global' object to store the business. Global only to the request.
	globalInfo := graph.NewAppenderGlobalInfo()
	globalInfo.Context = o.Context
	globalInfo.Business = business
	globalInfo.PromClient = prom
	// 这个是buildNamespaces TrafficMap
	trafficMap := istio.BuildNamespacesTrafficMap(o.TelemetryOptions, prom, globalInfo, span)
	genSpan := opentracing.StartSpan("generate", opentracing.FollowsFrom(span.Context()))
	code, config = generateGraph(trafficMap, o)
	genSpan.Finish()
	return code, config
}

// graphNodeIstio provides a test hook that accepts mock clients 获取节点的信息 和namespace有点不一样
func graphNodeIstio(business *business.Layer, client *prometheus.Client, o graph.Options) (code int, config interface{}) {

	// Create a 'global' object to store the business. Global only to the request.
	globalInfo := graph.NewAppenderGlobalInfo()
	globalInfo.Business = business
	globalInfo.PromClient = client
	//BuildNode TrafficMap
	trafficMap := istio.BuildNodeTrafficMap(o.TelemetryOptions, client, globalInfo)
	code, config = generateGraph(trafficMap, o)

	return code, config
}

func generateGraph(trafficMap graph.TrafficMap, o graph.Options) (int, cytoscape.Config) {
	log.Tracef("Generating config for [%s] graph...", o.ConfigVendor)

	promtimer := internalmetrics.GetGraphMarshalTimePrometheusTimer(o.GetGraphKind(), o.TelemetryOptions.GraphType, o.InjectServiceNodes)
	defer promtimer.ObserveDuration()

	var vendorConfig cytoscape.Config
	switch o.ConfigVendor {
	case graph.VendorCytoscape:
		vendorConfig = cytoscape.NewConfig(trafficMap, o.ConfigOptions)
	default:
		graph.Error(fmt.Sprintf("ConfigVendor [%s] not supported", o.ConfigVendor))
	}

	log.Tracef("Done generating config for [%s] graph", o.ConfigVendor)
	return http.StatusOK, vendorConfig
}
