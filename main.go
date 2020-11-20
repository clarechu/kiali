package main

import (
	"flag"
	"github.com/go-chi/chi"
	"github.com/golang/glog"
	"github.com/kiali/kiali/config"
	_ "github.com/kiali/kiali/docs"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/routers"
	"github.com/kiali/kiali/util"
	"github.com/spf13/cobra"
	"net/http"
	"os"
)

func init() {
	//flag.Parse()
	_ = flag.Set("v", "5")
	_ = flag.Set("logtostderr", "true")
	rootCmd.PersistentFlags().StringVar(&kiali.Context, "context",
		"cluster01", "当前集群的集群名称")
	rootCmd.PersistentFlags().StringVar(&kiali.Port, "port",
		":8000", "kiali 暴露端口地址 8000")
	rootCmd.PersistentFlags().StringVar(&kiali.PrometheusURL, "prometheus",
		"http://prometheus.istio-system:9090", "[prometheus api 接口 地址]")
	rootCmd.PersistentFlags().StringVar(&kiali.JaegerURL, "jaeger",
		"http://jaeger:9090", "[prometheus api 接口 地址]")

}

type Kiali struct {
	Port          string `json:"port"`
	JaegerURL     string `json:"jaeger_url"`
	PrometheusURL string `json:"prometheus_url"`
	Context       string `json:"context"`
}

var (

	// Command line arguments
	argConfigFile = flag.String("config", "", "Path to the YAML configuration file. If not specified, environment variables will be used for configuration.")
	kiali         = &Kiali{
		Port:          ":8000",
		Context:       "cluster01",
		JaegerURL:     "http://jaeger.service-mesh:6831",
		PrometheusURL: "http://prometheus.istio-system:9090",
	}

	rootCmd = &cobra.Command{
		Use:   "kiali",
		Short: "Kiali.",
		Long:  "kiali graph api version v1.0",
		Args:  cobra.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) (err error) {
			defer glog.Flush()
			util.Clock = util.RealClock{}
			config.Set(config.NewConfig())
			log.Tracef("Kiali Configuration:\n%+v", config.Get().Server.Address)
			r, err := kiali.NewServer()
			if err != nil {
				log.Errorf("new server error:%v", err)
				return
			}
			return kiali.Start(r)
		},
	}
)

// @title Swagger Kiali API
// @version 1.0
// @description This is a sample server Petstore server.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8000
// @BasePath /
// process command line
// load config file if specified, otherwise, rely on environment variables to configure us
//kiali --port :8000 --jaeger 10.10.13.30:26034 --prometheus http://10.10.13.30:9090
func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(-1)
	}
}

func (k *Kiali) NewServer() (*chi.Mux, error) {
	err := routers.InitOpentracing(k.JaegerURL)
	if err != nil {
		return nil, err
	}
	//return http.ListenAndServe(":8000", r)
	return routers.NewRouter(k.PrometheusURL, k.Context)
}

func (k *Kiali) Start(r *chi.Mux) error {
	log.Infof("server start http://localhost%s", k.Port)
	return http.ListenAndServe(k.Port, r)
}
