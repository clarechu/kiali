package main

import (
	"flag"
	"fmt"
	"github.com/golang/glog"
	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/util"
	"net/http"
)

func init() {
	// log everything to stderr so that it can be easily gathered by logs, separate log files are problematic with containers
	_ = flag.Set("logtostderr", "true")
	flag.Set("v", "5")

}

func main() {
	defer glog.Flush()
	util.Clock = util.RealClock{}

	// process command line
	flag.Parse()
	validateFlags()
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
	log.Errorf("server start %v", NewServer())

}

func NewServer() error {
	http.HandleFunc("/", IndexHandler)
	return http.ListenAndServe(":8000", nil)
}

func IndexHandler(w http.ResponseWriter, r *http.Request)  {
	fmt.Fprintln(w, "hello world")

	w.Write([]byte("HELLO WORLD!"))
}