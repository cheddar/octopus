package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	commonClients "github.com/tidepool-org/go-common/clients"
	"github.com/tidepool-org/go-common/clients/shoreline"

	"./../clients"
)

type (
	Api struct {
		Store            clients.StoreClient
		ShorelineClient  ShorelineInterface
		SeagullClient    SeagullInterface
		GatekeeperClient GatekeeperInterface
		Config           Config
	}
	Config struct {
		ServerSecret string `json:"serverSecret"` //used for services
		LongTermKey  string `json:"longTermKey"`
		Salt         string `json:"salt"`      //used for pw
		Secret       string `json:"apiSecret"` //used for token
	}

	GatekeeperInterface interface {
		UserInGroup(userID, groupID string) (map[string]commonClients.Permissions, error)
	}

	SeagullInterface interface {
		GetPrivatePair(userID, hashName, token string) *commonClients.PrivatePair
	}

	ShorelineInterface interface {
		CheckToken(token string) *shoreline.TokenData
		TokenProvide() string
	}

	httpVars map[string]string

	varsHandler func(http.ResponseWriter, *http.Request, httpVars)
)

func (a *Api) userCanViewData(userID, groupID string) bool {
	if userID == groupID {
		return true
	}

	perms, err := a.GatekeeperClient.UserInGroup(userID, groupID)
	if err != nil {
		log.Println("Error looking up user in group", err)
		return false
	}

	log.Println(perms)
	return !(perms["root"] == nil && perms["view"] == nil)
}

func InitApi(cfg Config, slc ShorelineInterface,
	sgc SeagullInterface, gkc GatekeeperInterface,
	store clients.StoreClient) *Api {

	return &Api{
		Store:            store,
		ShorelineClient:  slc,
		SeagullClient:    sgc,
		GatekeeperClient: gkc,
		Config:           cfg,
	}
}

func (a *Api) SetHandlers(prefix string, rtr *mux.Router) {
	rtr.HandleFunc("/status", a.GetStatus).Methods("GET")
	rtr.Handle("/upload/lastentry/{userID}", varsHandler(a.TimeLastEntryUser)).Methods("GET")
	rtr.Handle("/upload/lastentry/{userID}/{deviceID}", varsHandler(a.TimeLastEntryUserAndDevice)).Methods("GET")
}

func (a *Api) GetStatus(res http.ResponseWriter, req *http.Request) {
	if err := a.Store.Ping(); err != nil {
		log.Println(err)
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte(err.Error()))
		return
	}
	res.WriteHeader(http.StatusOK)
	res.Write([]byte("OK"))
}

func (a *Api) authorizeAndGetGroupId(res http.ResponseWriter, req *http.Request, vars httpVars) (string, error) {
	userID := vars["userID"]
	token := req.Header.Get("x-tidepool-session-token")

	if td := a.ShorelineClient.CheckToken(token); td != nil {
		fmt.Println("td.UserID", td.UserID, "userID", userID)
	}

	if td == nil || !(td.IsServer || a.userCanViewData(td.UserID, userID)) {
		res.WriteHeader(http.StatusForbidden)
		return "fail", errors.New("Forbidden")
	}

	if pair := a.SeagullClient.GetPrivatePair(userID, "uploads", a.ShorelineClient.TokenProvide()); pair == nil {
		res.WriteHeader(http.StatusInternalServerError)
		return "fail", errors.New("Internal server error")
	}

	return pair.ID, nil
}

// http.StatusOK,  time of last entry
func (a *Api) TimeLastEntryUser(res http.ResponseWriter, req *http.Request, vars httpVars) {
	if groupId, err := a.authorizeAndGetGroupId(res, req, vars); err != nil {
		return
	}
	timeLastEntry := a.Store.GetTimeLastEntryUser(groupId)
	res.WriteHeader(http.StatusOK)
	res.Write(timeLastEntry)
}

// http.StatusOK, time of last entry and device
func (a *Api) TimeLastEntryUserAndDevice(res http.ResponseWriter, req *http.Request, vars httpVars) {
	if groupId, err := a.authorizeAndGetGroupId(res, req, vars); err != nil {
		return
	}

	deviceId := vars["deviceID"]

	timeLastEntry := a.Store.GetTimeLastEntryUserAndDevice(groupId, deviceId)

	res.WriteHeader(http.StatusOK)
	res.Write(timeLastEntry)
}

func (h varsHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	h(res, req, vars)
}
