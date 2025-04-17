package router

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ArmaanKatyal/porta/pkg/auth"
	"github.com/ArmaanKatyal/porta/pkg/config"
	"github.com/ArmaanKatyal/porta/pkg/observability"
	"github.com/ArmaanKatyal/porta/pkg/proxy"
	"github.com/ArmaanKatyal/porta/pkg/util"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sony/gobreaker/v2"
)

type RequestHandler struct {
	ServiceRegistry *proxy.ServiceRegistry
	Metrics         *observability.PromMetrics
}

func NewRequestHandler() *RequestHandler {
	m := observability.NewPromMetrics()
	return &RequestHandler{
		ServiceRegistry: proxy.NewServiceRegistry(m),
		Metrics:         m,
	}
}

func GetStatusCode(statusCode int) string {
	return string(rune(statusCode))
}

// Health is a simple health check endpoint
func Health(w http.ResponseWriter, r *http.Request) {
	slog.Info("Health check", "req", util.RequestToMap(r))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		slog.Error("Error writing response", "error", err.Error())
	}
}

// Config returns the application configuration
func Config(w http.ResponseWriter, r *http.Request) {
	slog.Info("Get config", "req", util.RequestToMap(r))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(config.AppConfig.Marshal()); err != nil {
		slog.Error("Error writing response", "error", err.Error())
	}
}

// InitializeRoutes initializes the application routes
func InitializeRoutes(r *RequestHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", r.HandleRequest)
	return mux
}

func InitializeAdminRoutes(r *RequestHandler) *http.ServeMux {
	go r.ServiceRegistry.Heartbeat()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /services/register", r.ServiceRegistry.RegisterService)
	mux.HandleFunc("POST /services/deregister", r.ServiceRegistry.DeregisterService)
	mux.HandleFunc("GET /services", r.ServiceRegistry.GetServices)
	mux.HandleFunc("POST /services/update", r.ServiceRegistry.UpdateService)
	mux.HandleFunc("GET /health", Health)
	mux.HandleFunc("GET /config", Config)
	mux.Handle("GET /metrics", promhttp.Handler())
	return mux
}

func (rh *RequestHandler) circuitBreakerEnabled(svc string) bool {
	return rh.ServiceRegistry.GetService(svc).CircuitBreaker.IsEnabled()
}

func (rh *RequestHandler) CollectMetrics(input *observability.MetricsInput, t time.Time) {
	rh.Metrics.Collect(input, t)
}

// resolvePath splits the path into service name and route path
func (rh *RequestHandler) resolvePath(path string) (string, []string) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return path, nil
	}
	return parts[1], parts[2:]
}

// createForwardURI creates a new uri based on the resolved request
func (rh *RequestHandler) createForwardURI(address string, route []string, query string) string {
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		address = "http://" + address
	}
	forwardUri := address + "/" + strings.Join(route, "/")
	if query != "" {
		forwardUri = forwardUri + "?" + query
	}
	return forwardUri
}

// HandleRequest handles the incoming request and forwards it to the resolved service
func (rh *RequestHandler) HandleRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	slog.Info("Received request", "req", util.RequestToMap(r))
	serviceName, route := rh.resolvePath(r.URL.Path)
	slog.Info("Resolving service", "service_name", serviceName)
	service := rh.ServiceRegistry.GetService(serviceName)
	if service == nil {
		slog.Error("No service exists with the provided name", "service", serviceName)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if service.IsRateLimiterEnabled() && !service.RateLimitIP(r.RemoteAddr) {
		slog.Error("Rate limit exceeded", "path", r.URL.Path, "method", r.Method, "ip", r.RemoteAddr, "service", serviceName)
		http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
		rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusTooManyRequests), Method: r.Method, Route: r.URL.String()}, start)
		return
	}
	if ok, err := service.IsWhitelisted(r.RemoteAddr); !ok || err != nil {
		slog.Error("Unauthorized request", "path", r.URL.Path, "method", r.Method, "ip", r.RemoteAddr, "service_name", serviceName)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusUnauthorized), Method: r.Method, Route: r.URL.String()}, start)
		return
	}

	if err := service.Authenticate(r); err != nil {
		// If Auth fails reject the request with an appropriate message and status code
		switch err {
		case auth.ErrTokenMissing:
			slog.Error("Auth failed", "service_name", serviceName, "error", err.Error())
			http.Error(w, "token missing", http.StatusUnauthorized)
			rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusUnauthorized), Method: r.Method, Route: r.URL.String()}, start)
			return
		case auth.ErrInvalidToken:
			slog.Error("Auth failed", "service_name", serviceName, "error", err.Error())
			http.Error(w, "invalid token", http.StatusUnauthorized)
			rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusUnauthorized), Method: r.Method, Route: r.URL.String()}, start)
			return
		default:
			slog.Error("Auth failed", "service_name", serviceName, "error", err.Error())
			http.Error(w, "auth failed", http.StatusUnauthorized)
			rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusUnauthorized), Method: r.Method, Route: r.URL.String()}, start)
			return
		}
	}

	if service.Addr == "" {
		slog.Error("Service not found", "service_name", serviceName)
		http.Error(w, "service not found", http.StatusNotFound)
		rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusNotFound), Method: r.Method, Route: r.URL.String()}, start)
		return
	}

	// Check cache for the service
	key := rh.generateCacheKey(serviceName, r)
	v, hit := service.Cache.Get(key)
	if service.Cache.IsEnabled() && hit {
		slog.Info("Cache hit", "service", serviceName, "path", r.URL.Path, "method", r.Method)
		switch value := v.(type) {
		case []byte:
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(value)
			if err != nil {
				slog.Error("Error writing response", "error", err.Error())
				http.Error(w, "error writing response", http.StatusInternalServerError)
				rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusInternalServerError), Method: r.Method, Route: r.URL.String()}, start)
				return
			}
			rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusOK), Method: r.Method, Route: r.URL.String()}, start)
			return
		default:
			slog.Error("Wrong type data from cache", "service", serviceName, "path", r.URL.Path)
			http.Error(w, "return data type mismatch", http.StatusInternalServerError)
			rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusInternalServerError), Method: r.Method, Route: r.URL.String()}, start)
			return
		}
	}

	// Create a new uri based on the resolved request
	forwardUri := rh.createForwardURI(service.Addr, route, r.URL.RawQuery)

	slog.Info("Forwarding request", "forward_uri", forwardUri, "service_name", serviceName)

	var err error
	// Forward the request with or without circuit breaker
	if rh.circuitBreakerEnabled(serviceName) {
		err = rh.forwardRequestCB(w, r, forwardUri, service.CircuitBreaker, serviceName, start)
	} else {
		err = rh.forwardRequest(w, r, forwardUri, serviceName, start)
	}
	if err != nil {
		slog.Error("Error forwarding request", "error", err.Error(), "service_name", serviceName)
		http.Error(w, "service is down", http.StatusInternalServerError)
		rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusInternalServerError), Method: r.Method, Route: r.URL.String()}, start)
	}
}

// generateCacheKey generates a key based on the service name and request.URL
// TODO: maybe also include request.Headers and hash them together to generate more cohesive key
func (rh *RequestHandler) generateCacheKey(service string, r *http.Request) string {
	headers := ""
	for k, v := range r.Header {
		headers += "[" + k + "-" + strings.Join(v, "-") + "]"
	}
	val, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("failed to parse req body while generating cache key", "service", service, "req", util.RequestToMap(r))
		val = []byte{}
	}
	components := []string{service, r.Method, r.URL.String(), headers, string(val)}
	baseKey := "cache-" + strings.Join(components, "-")
	h := sha256.New()
	h.Write([]byte(baseKey))
	return string(h.Sum(nil))
}

// forwardRequest forwards the request to the resolved service
func (rh *RequestHandler) forwardRequest(w http.ResponseWriter, r *http.Request, forwardUri string, service string, t time.Time) error {
	req, err := http.NewRequest(r.Method, forwardUri, r.Body)
	if err != nil {
		rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusInternalServerError), Method: r.Method, Route: r.URL.String()}, t)
		return err
	}
	req.Header = cloneHeader(r.Header)

	// add a unique trace id to every request for tracing
	req.Header.Add("X-Trace-Id", uuid.NewString())
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusInternalServerError), Method: r.Method, Route: r.URL.String()}, t)
		return err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	// Copy the response from the resolved service
	copyResponseHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		return err
	}

	// Save the response in the cache
	val, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	key := rh.generateCacheKey(service, r)
	if ok := rh.ServiceRegistry.SetCache(service, key, val); !ok {
		slog.Error("error setting value in cache", "service", service, "path", r.URL.String(), "key", key)
		return errors.New("SetCache failed")
	}
	slog.Info("SetCache successful", "service", service, "path", r.URL.String(), "key", key)

	rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(resp.StatusCode), Method: r.Method, Route: r.URL.String()}, t)
	return nil
}

// cloneHeader clones the header
func cloneHeader(h http.Header) http.Header {
	cloned := make(http.Header, len(h))
	for k, v := range h {
		cloned[k] = append([]string(nil), v...)
	}
	return cloned
}

// copyResponseHeaders copies the response headers
func copyResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
}

// forwardRequestCB forwards the request to the resolved service with circuit breaker
func (rh *RequestHandler) forwardRequestCB(w http.ResponseWriter, r *http.Request, forwardURI string, cb proxy.ICircuitBreaker, service string, t time.Time) error {
	// Define the request execution function
	executeRequest := func() ([]byte, error) {
		// Create a new request
		req, err := http.NewRequest(r.Method, forwardURI, r.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create new request: %w", err)
		}

		// Copy headers from the original request and add a trace ID
		req.Header = cloneHeader(r.Header)
		req.Header.Add("X-Trace-Id", uuid.NewString())

		// Execute the request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request execution failed: %w", err)
		}
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(resp.Body)

		// Copy response headers and status code
		copyResponseHeaders(w, resp)
		w.WriteHeader(resp.StatusCode)

		// Read the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
		return body, nil
	}

	// Execute the request with the circuit breaker
	body, err := cb.Execute(service, executeRequest)
	if err != nil {
		// Handle the case where the circuit is open and fallback is needed
		if cb.IsOpen() || errors.Is(err, gobreaker.ErrOpenState) {
			return rh.handleFallbackRequest(w, r, service, t)
		}
		return err
	}

	// Write the response body
	_, err = w.Write(body)
	if err != nil {
		return fmt.Errorf("failed to write response body: %w", err)
	}

	// Save the response in the cache
	key := rh.generateCacheKey(service, r)
	if ok := rh.ServiceRegistry.SetCache(service, key, body); !ok {
		slog.Error("error setting value in cache", "service", service, "path", r.URL.String(), "key", key)
		return errors.New("SetCache failed")
	}
	slog.Info("SetCache successful cb", "service", service, "path", r.URL.String(), "key", key)

	rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusOK), Method: r.Method, Route: r.URL.String()}, t)
	return nil
}

// handleFallbackRequest handles the case where the circuit breaker is open and a fallback request is needed
func (rh *RequestHandler) handleFallbackRequest(w http.ResponseWriter, r *http.Request, service string, t time.Time) error {
	slog.Error("Circuit breaker is open, making a fallback request", "service", service)
	fallbackURI := rh.ServiceRegistry.GetFallbackUri(service)
	if fallbackURI == "" {
		// If fallbackURI is not provided the default behavior is to return a 503
		slog.Info("no fallbackURI provided", "service", service)
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		rh.CollectMetrics(&observability.MetricsInput{Code: GetStatusCode(http.StatusServiceUnavailable), Method: r.Method, Route: r.URL.String()}, t)
		return nil
	}

	// Resolve the path and create a new URI
	_, route := rh.resolvePath(r.URL.Path)
	forwardURI := rh.createForwardURI(fallbackURI, route, r.URL.RawQuery)
	// Forward the request
	return rh.forwardRequest(w, r, forwardURI, service, t)
}
