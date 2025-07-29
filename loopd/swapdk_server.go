package loopd

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/lightninglabs/loop/swapdk"
)

// swapDKServer is the HTTP server that handles requests from the SwapDK
// clients.
type swapDKServer struct {
	ctx    context.Context
	router *mux.Router
	server *swapdk.SwapDKService
}

// newSwapDKServer creates a new swapDKServer.
func newSwapDKServer(server *swapdk.SwapDKService) *swapDKServer {
	s := &swapDKServer{
		ctx:    context.Background(),
		router: mux.NewRouter(),
		server: server,
	}

	s.router.HandleFunc("/v1/swapdk/register", s.registerClient).Methods("POST")
	s.router.HandleFunc("/v1/swapdk/events", s.getEvents).Methods("GET")
	s.router.HandleFunc("/v1/swapdk/events/{id}/respond", s.respondToEvent).Methods("POST")
	s.router.HandleFunc("/v1/swapdk/balance", s.getBalance).Methods("GET")
	s.router.HandleFunc("/v1/swapdk/transactions", s.getTransactions).Methods("GET")

	return s
}

// registerClient handles the registration of a new client.
func (s *swapDKServer) registerClient(w http.ResponseWriter, r *http.Request) {
	// To be implemented.
	w.WriteHeader(http.StatusNotImplemented)
}

// getEvents handles the fetching of pending signing events.
func (s *swapDKServer) getEvents(w http.ResponseWriter, r *http.Request) {
	requests := s.server.GetPendingSignatureRequests()

	json.NewEncoder(w).Encode(requests)
}

// respondToEvent handles the response to a signing event.
func (s *swapDKServer) respondToEvent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	var id [32]byte
	copy(id[:], []byte(idStr))

	var resp swapdk.SigningResponse
	err := json.NewDecoder(r.Body).Decode(&resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = s.server.RespondToSignatureRequest(id, &resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// getBalance handles the fetching of the static address balance.
func (s *swapDKServer) getBalance(w http.ResponseWriter, r *http.Request) {
	balance, err := s.server.GetBalance(s.ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(balance)
}

// getTransactions handles the fetching of the static address transactions.
func (s *swapDKServer) getTransactions(w http.ResponseWriter, r *http.Request) {
	txs, err := s.server.GetTransactions(s.ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(txs)
}
