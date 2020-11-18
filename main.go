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
	"net/http"
)

func init() {
	// log everything to stderr so that it can be easily gathered by logs, separate log files are problematic with containers
	_ = flag.Set("logtostderr", "true")
	_ = flag.Set("v", "5")

}

// Command line arguments
var (
	argConfigFile = flag.String("config", "", "Path to the YAML configuration file. If not specified, environment variables will be used for configuration.")
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
func main() {
	defer glog.Flush()
	util.Clock = util.RealClock{}

	// process command line
	flag.Parse()
	// load config file if specified, otherwise, rely on environment variables to configure us
	if *argConfigFile != "" {
		c, err := config.LoadFromFile(*argConfigFile)
		if err != nil {
			glog.Fatal(err)
		}
		config.Set(c)
	} else {
		log.Infof("No configuration file specified. Will rely on environment for configuration.")
		config.Set(config.NewConfig())
	}

	log.Tracef("Kiali Configuration:\n%+v", config.Get().Server.Address)
	r, err := NewServer()
	if err != nil {
		log.Errorf("new server error:%v", err)
		return
	}
	Start(r)
}

func NewServer() (*chi.Mux, error) {
	err := routers.InitOpentracing("10.10.13.30:26034")
	if err != nil {
		return nil, err
	}
	//return http.ListenAndServe(":8000", r)
	return routers.NewRouter()
}

func Start(r *chi.Mux) {
	log.Errorf("server start %v", http.ListenAndServe(":8000", r))
}
