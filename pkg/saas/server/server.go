package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bacalhau-project/generic-dcn/pkg/saas/controller"
	"github.com/bacalhau-project/generic-dcn/pkg/saas/store"
	"github.com/bacalhau-project/generic-dcn/pkg/saas/system"
	"github.com/gorilla/mux"
)

type ServerOptions struct {
	URL           string
	Host          string
	Port          int
	KeyCloakURL   string
	KeyCloakToken string
	// this is for when we are running localfs filesystem
	// and we need to add a route to view files based on their path
	// we are assuming all file storage is open right now
	// so we just deep link to the object path and don't apply auth
	// (this is so lilypad nodes can see files)
	// later, we might add a token to the URLs
	LocalFilestorePath string
}

type LilysaasAPIServer struct {
	Options    ServerOptions
	Store      store.Store
	Controller *controller.Controller
}

func NewServer(
	options ServerOptions,
	store store.Store,
	controller *controller.Controller,
) (*LilysaasAPIServer, error) {
	if options.URL == "" {
		return nil, fmt.Errorf("server url is required")
	}
	if options.Host == "" {
		return nil, fmt.Errorf("server host is required")
	}
	if options.Port == 0 {
		return nil, fmt.Errorf("server port is required")
	}
	if options.KeyCloakURL == "" {
		return nil, fmt.Errorf("keycloak url is required")
	}
	if options.KeyCloakToken == "" {
		return nil, fmt.Errorf("keycloak token is required")
	}

	return &LilysaasAPIServer{
		Options:    options,
		Store:      store,
		Controller: controller,
	}, nil
}

func (apiServer *LilysaasAPIServer) ListenAndServe(ctx context.Context, cm *system.CleanupManager) error {
	router := mux.NewRouter()
	router.Use(apiServer.corsMiddleware)

	subrouter := router.PathPrefix("/api/v1").Subrouter()

	// add one more subrouter for the authenticated service methods
	authRouter := subrouter.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		return true
	}).Subrouter()

	keycloak := newKeycloak(apiServer.Options)
	keyCloakMiddleware := newMiddleware(keycloak, apiServer.Options, apiServer.Store)
	authRouter.Use(keyCloakMiddleware.verifyToken)

	subrouter.HandleFunc("/modules", wrapper(apiServer.getModules)).Methods("GET")

	authRouter.HandleFunc("/status", wrapper(apiServer.status)).Methods("GET")
	authRouter.HandleFunc("/jobs", wrapper(apiServer.getJobs)).Methods("GET")
	authRouter.HandleFunc("/transactions", wrapper(apiServer.getTransactions)).Methods("GET")

	// TODO: make this route work when not logged in (is this the most important
	// thing to do next??) probably not. users like fluence can use their
	// logged-in api token and we'll just give them $1M of credits
	authRouter.HandleFunc("/jobs/async", wrapper(apiServer.createJobAsync)).Methods("POST")
	authRouter.HandleFunc("/jobs/sync", wrapper(apiServer.createJobSync)).Methods("POST")

	authRouter.HandleFunc("/filestore/config", wrapper(apiServer.filestoreConfig)).Methods("GET")
	authRouter.HandleFunc("/filestore/list", wrapper(apiServer.filestoreList)).Methods("GET")
	authRouter.HandleFunc("/filestore/get", wrapper(apiServer.filestoreGet)).Methods("GET")
	authRouter.HandleFunc("/filestore/folder", wrapper(apiServer.filestoreCreateFolder)).Methods("POST")
	authRouter.HandleFunc("/filestore/upload", wrapper(apiServer.filestoreUpload)).Methods("POST")
	authRouter.HandleFunc("/filestore/rename", wrapper(apiServer.filestoreRename)).Methods("PUT")
	authRouter.HandleFunc("/filestore/delete", wrapper(apiServer.filestoreDelete)).Methods("DELETE")

	authRouter.HandleFunc("/api_keys", wrapper(apiServer.createAPIKey)).Methods("POST")
	authRouter.HandleFunc("/api_keys", wrapper(apiServer.getAPIKeys)).Methods("GET")
	authRouter.HandleFunc("/api_keys", wrapper(apiServer.deleteAPIKey)).Methods("DELETE")
	authRouter.HandleFunc("/api_keys/check", wrapper(apiServer.checkAPIKey)).Methods("GET")

	if apiServer.Options.LocalFilestorePath != "" {
		fileServer := http.FileServer(http.Dir(apiServer.Options.LocalFilestorePath))
		subrouter.PathPrefix("/filestore/viewer/").Handler(http.StripPrefix("/api/v1/filestore/viewer/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fileServer.ServeHTTP(w, r)
		})))
	}

	StartWebSocketServer(
		ctx,
		subrouter,
		"/ws",
		apiServer.Controller.JobUpdatesChan,
		keyCloakMiddleware.userIDFromRequest,
	)

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", apiServer.Options.Host, apiServer.Options.Port),
		WriteTimeout:      time.Minute * 15,
		ReadTimeout:       time.Minute * 15,
		ReadHeaderTimeout: time.Minute * 15,
		IdleTimeout:       time.Minute * 60,
		Handler:           router,
	}
	return srv.ListenAndServe()
}
