package chat

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	h "github.com/tonto/kit/http"
	"github.com/tonto/kit/http/respond"
)

const (
	defMaxLen = 20

	minNickLen       = 3
	maxNickLen       = 20
	minNickSecretLen = 5
	maxNickSecretLen = 30

	minChanNameLen   = 3
	maxChanNameLen   = 25
	maxChanSecretLen = 64
)

// NewAPI creates new websocket api
func NewAPI(store Store, admin, password string) *API {
	api := API{
		store: store,
	}

	api.RegisterEndpoint(
		"POST",
		"/admin/create_channel",
		api.createChannel,
		WithHTTPBasicAuth(admin, password),
	)

	api.RegisterEndpoint(
		"POST",
		"/admin/unread_count",
		api.unreadCount,
		WithHTTPBasicAuth(admin, password),
	)

	api.RegisterHandler("GET", "/list_channels", api.listChannels)
	api.RegisterEndpoint("POST", "/register_nick", api.registerNick)
	api.RegisterEndpoint("POST", "/channel_members", api.channelMembers)

	return &api
}

// API represents websocket api service
type API struct {
	h.BaseService
	store Store
}

// Store represents chat store interface
type Store interface {
	Save(*Chat) error
	Get(string) (*Chat, error)
	ListChannels() ([]string, error)
	GetUnreadCount(string, string) uint64
}

// Prefix returns api prefix for this service
func (api *API) Prefix() string { return "chat" }

type createChanReq struct {
	Name    string `json:"name"`
	Private bool   `json:"private"`
}

type createChanResp struct {
	Secret string `json:"secret"`
}

func (cr *createChanReq) Validate() error {
	if cr.Name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if len(cr.Name) < minChanNameLen || len(cr.Name) > maxChanNameLen {
		return fmt.Errorf("name must be between %d and %d characters long", minChanNameLen, maxChanNameLen)
	}
	if match, err := regexp.Match("^[a-zA-Z0-9_]*$", []byte(cr.Name)); !match || err != nil {
		return fmt.Errorf("name must contain only alphanumeric and underscores")
	}
	return nil
}

func (api *API) createChannel(c context.Context, w http.ResponseWriter, req *createChanReq) (*h.Response, error) {
	ch := NewChannel(req.Name, req.Private)
	if err := api.store.Save(ch); err != nil {
		return nil, fmt.Errorf("could not create channel at this moment")
	}
	return h.NewResponse(createChanResp{Secret: ch.Secret}, http.StatusOK), nil
}

type registerNickReq struct {
	Nick          string `json:"nick"`
	FullName      string `json:"name"`
	Email         string `json:"email"`
	Secret        string `json:"secret"`
	Channel       string `json:"channel"`
	ChannelSecret string `json:"channel_secret"` // Tennant
}

type registerNickResp struct {
	Secret string `json:"secret"`
}

func (r *registerNickReq) Validate() error {
	if r.Nick == "" {
		return fmt.Errorf("nick is required")
	}
	if r.Channel == "" {
		return fmt.Errorf("channel is required")
	}
	if len(r.Nick) < minNickLen || len(r.Nick) > maxNickLen {
		return fmt.Errorf("nick must be between %d and %d characters long", minNickLen, maxNickLen)
	}
	if match, err := regexp.Match("^[a-zA-Z0-9_]*$", []byte(r.Nick)); !match || err != nil {
		return fmt.Errorf("nick must contain only alphanumeric and underscores")
	}
	if len(r.FullName) > defMaxLen || len(r.Email) > defMaxLen {
		return fmt.Errorf("exceeded max field length of %d", defMaxLen)
	}
	if len(r.ChannelSecret) > maxChanSecretLen {
		return fmt.Errorf("exceeded max channel secret length of %d", maxChanSecretLen)
	}
	if r.Secret != "" && (len(r.Secret) < minNickSecretLen || len(r.Secret) > maxNickSecretLen) {
		return fmt.Errorf("secret should be between %d and %d characters long", minNickSecretLen, maxNickSecretLen)
	}
	if match, err := regexp.Match("^[a-zA-Z0-9_]*$", []byte(r.Secret)); r.Secret != "" && !match || err != nil {
		return fmt.Errorf("secret must contain only alphanumeric and underscores")
	}
	return nil
}

func (api *API) registerNick(c context.Context, w http.ResponseWriter, req *registerNickReq) (*h.Response, error) {
	ch, err := api.store.Get(req.Channel)
	if err != nil {
		return nil, fmt.Errorf("could not fetch channel")
	}

	if ch.Secret != req.ChannelSecret {
		return nil, fmt.Errorf("invalid secret")
	}

	secret, err := ch.Register(&User{
		Nick:     req.Nick,
		FullName: req.FullName,
		Email:    req.Email,
	}, req.Secret)

	if err != nil {
		return nil, err
	}

	// TODO - Need transaction
	err = api.store.Save(ch)
	if err != nil {
		return nil, fmt.Errorf("could not update channel membership")
	}

	return h.NewResponse(registerNickResp{Secret: secret}, http.StatusOK), nil
}

type unreadCountReq struct {
	Channel string `json:"channel"`
	Nick    string `json:"nick"`
}

func (r *unreadCountReq) Validate() error {
	if r.Nick == "" {
		return fmt.Errorf("nick is required")
	}
	if len(r.Nick) < minNickLen || len(r.Nick) > maxNickLen {
		return fmt.Errorf("nick must be between %d and %d characters long", minNickLen, maxNickLen)
	}
	if r.Channel == "" {
		return fmt.Errorf("channel is required")
	}
	if len(r.Channel) > maxChanNameLen {
		return fmt.Errorf("channel name must not exceed %d characters", maxChanNameLen)
	}
	return nil
}

func (api *API) unreadCount(c context.Context, w http.ResponseWriter, req *unreadCountReq) (*h.Response, error) {
	return h.NewResponse(api.store.GetUnreadCount(req.Nick, req.Channel), http.StatusOK), nil
}

type channelMembersReq struct {
	Channel       string `json:"channel"`
	ChannelSecret string `json:"channel_secret"`
}

func (r *channelMembersReq) Validate() error {
	if r.Channel == "" {
		return fmt.Errorf("channel is required")
	}
	if len(r.Channel) > maxChanNameLen {
		return fmt.Errorf("channel name must not exceed %d characters", maxChanNameLen)
	}
	if len(r.ChannelSecret) > maxChanSecretLen {
		return fmt.Errorf("channel_secret must not exceed %d characters", maxChanSecretLen)
	}
	return nil
}

func (api *API) channelMembers(c context.Context, w http.ResponseWriter, req *channelMembersReq) (*h.Response, error) {
	ch, err := api.store.Get(req.Channel)
	if err != nil {
		return nil, fmt.Errorf("could not fetch channel")
	}

	members := []User{}

	if ch.Members != nil && len(ch.Members) > 0 {
		for _, u := range ch.Members {
			u.Secret = ""
			members = append(members, u)
		}
	}

	return h.NewResponse(members, http.StatusOK), nil
}

func (api *API) listChannels(c context.Context, w http.ResponseWriter, r *http.Request) {
	chans, err := api.store.ListChannels()
	if err != nil {
		respond.WithJSON(
			w, r,
			h.NewError(http.StatusInternalServerError, err),
		)
		return
	}
	respond.WithJSON(
		w, r,
		chans,
	)
}
