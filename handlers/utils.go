package handlers

import (
	"errors"
	"github.com/opentracing/opentracing-go"
	"k8s.io/client-go/rest"
	"net/http"
	"net/url"

	"github.com/kiali/kiali/business"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/models"
	"github.com/kiali/kiali/prometheus"
)

type promClientSupplier func() (*prometheus.Client, error)

type promNoAuthClientSupplier func(promAddress string) (*prometheus.Client, error)

var defaultPromClientSupplier = prometheus.NewClient

var DefaultNoAuthPromClientSupplier = prometheus.NewClientNoAuth

func validateURL(serviceURL string) (*url.URL, error) {
	return url.ParseRequestURI(serviceURL)
}

func checkNamespaceAccess(w http.ResponseWriter, r *http.Request, prom prometheus.ClientInterface, namespace string) *models.Namespace {
	layer, err := getBusiness(r)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, err.Error())
		return nil
	}

	if nsInfo, err := layer.Namespace.GetNamespace(namespace); err != nil {
		RespondWithError(w, http.StatusForbidden, "Cannot access namespace data: "+err.Error())
		return nil
	} else {
		return nsInfo
	}
}

func CheckNamespaceAccess(config *rest.Config, promAddress string, namespace string) *models.Namespace {
	layer, err := GetBusinessNoAuth(config, promAddress, nil)
	if err != nil {
		return nil
	}

	if nsInfo, err := layer.Namespace.GetNoCacheNamespace(namespace); err != nil {
		return nil
	} else {
		return nsInfo
	}
}

func InitContextClientsForMetrics(promSupplier promNoAuthClientSupplier, namespace string,
	config *rest.Config, promAddress string) (*prometheus.Client, *models.Namespace) {
	prom, err := promSupplier(promAddress)
	if err != nil {
		return nil, nil
	}

	nsInfo := CheckNamespaceAccess(config, promAddress, namespace)
	if nsInfo == nil {
		return nil, nil
	}
	return prom, nsInfo
}

func initClientsForMetrics(w http.ResponseWriter, r *http.Request, promSupplier promClientSupplier, namespace string) (*prometheus.Client, *models.Namespace) {
	prom, err := promSupplier()
	if err != nil {
		return nil, nil
	}

	nsInfo := checkNamespaceAccess(w, r, prom, namespace)
	if nsInfo == nil {
		return nil, nil
	}
	return prom, nsInfo
}

func InitClientsForMetrics(promSupplier promNoAuthClientSupplier, namespace string,
	config *rest.Config, promAddress string) (*prometheus.Client, *models.Namespace) {
	prom, err := promSupplier(promAddress)
	if err != nil {
		log.Error(err)
		return nil, nil
	}

	nsInfo := CheckNamespaceAccess(config, promAddress, namespace)
	if nsInfo == nil {
		return nil, nil
	}
	return prom, nsInfo
}

// getToken retrieves the token from the request's context
func getToken(r *http.Request) (string, error) {
	tokenContext := r.Context().Value("token")
	if tokenContext != nil {
		if token, ok := tokenContext.(string); ok {
			return token, nil
		} else {
			return "", errors.New("token is not of type string")
		}
	} else {
		return "", errors.New("token missing from the request context")
	}
}

// getBusiness returns the business layer specific to the users's request
func getBusiness(r *http.Request) (*business.Layer, error) {
	token, err := getToken(r)
	if err != nil {
		return nil, err
	}
	return business.Get(token)
}

// getBusiness noauth returns the business layer specific to the users's request
func GetBusinessNoAuth(config *rest.Config, promAddress string, ctx opentracing.SpanContext) (*business.Layer, error) {
	span := opentracing.StartSpan("get auth", opentracing.ChildOf(ctx))
	return business.GetNoAuth(config, promAddress, span)
}
