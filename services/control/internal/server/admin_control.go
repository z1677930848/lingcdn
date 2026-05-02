package server

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/lingcdn/control/internal/config"
	"github.com/lingcdn/control/internal/publisher"
	"github.com/lingcdn/control/internal/purge"
	"github.com/lingcdn/control/internal/store"
	controlpb "github.com/lingcdn/control/proto/gen"
)

var domainLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type adminControlServer struct {
	controlpb.UnimplementedAdminControlServer
	cfg       config.Config
	store     store.Store
	publisher *publisher.Publisher
	purge     *purge.Service
}

func newAdminControlServer(cfg config.Config, store store.Store, publisher *publisher.Publisher, purge *purge.Service) *adminControlServer {
	return &adminControlServer{
		cfg:       cfg,
		store:     store,
		publisher: publisher,
		purge:     purge,
	}
}

func (s *adminControlServer) CreateDomain(ctx context.Context, req *controlpb.CreateDomainRequest) (*controlpb.CreateDomainResponse, error) {
	if err := requireServiceToken(ctx, s.cfg.ServiceToken); err != nil {
		return nil, err
	}
	if req == nil || req.GetDomain() == nil {
		return nil, status.Error(codes.InvalidArgument, "domain required")
	}

	in := req.GetDomain()
	name, err := normalizeDomainName(in.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	originID := strings.TrimSpace(in.GetOriginId())
	if originID == "" {
		return nil, status.Error(codes.InvalidArgument, "origin_id required")
	}

	origin, err := s.store.GetOrigin(ctx, originID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Ctx(ctx).Error().Err(err).Str("origin_id", originID).Msg("failed to load origin")
		return nil, status.Errorf(codes.Internal, "failed to validate origin %q: %v", originID, err)
	}
	if origin == nil {
		return nil, status.Error(codes.NotFound, "origin not found")
	}

	existing, err := s.store.GetDomainByName(ctx, name)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Ctx(ctx).Error().Err(err).Str("name", name).Msg("failed to check domain existence")
		return nil, status.Errorf(codes.Internal, "failed to check domain existence for %q: %v", name, err)
	}
	if existing != nil {
		return nil, status.Error(codes.AlreadyExists, "domain already exists")
	}

	domainID := strings.TrimSpace(in.GetId())
	if domainID == "" {
		domainID = uuid.NewString()
	}

	now := time.Now()
	newDomain := &store.Domain{
		ID:        domainID,
		Name:      name,
		OriginID:  originID,
		CertID:    "",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.store.CreateDomain(ctx, newDomain); err != nil {
		log.Ctx(ctx).Error().Err(err).Str("name", name).Msg("failed to create domain")
		return nil, status.Errorf(codes.Internal, "failed to insert domain %q: %v", name, err)
	}

	log.Ctx(ctx).Info().
		Str("id", domainID).
		Str("name", name).
		Str("origin_id", originID).
		Msg("domain created")

	return &controlpb.CreateDomainResponse{
		Domain: &controlpb.Domain{
			Id:       domainID,
			Name:     name,
			OriginId: originID,
		},
	}, nil
}

func (s *adminControlServer) ListDomains(ctx context.Context, req *controlpb.ListDomainsRequest) (*controlpb.ListDomainsResponse, error) {
	if err := requireServiceToken(ctx, s.cfg.ServiceToken); err != nil {
		return nil, err
	}
	_ = req

	domains, err := s.store.ListDomains(ctx)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("failed to list domains")
		return nil, status.Errorf(codes.Internal, "failed to list domains: %v", err)
	}

	resp := &controlpb.ListDomainsResponse{Domains: make([]*controlpb.Domain, 0, len(domains))}
	for _, d := range domains {
		resp.Domains = append(resp.Domains, &controlpb.Domain{
			Id:       d.ID,
			Name:     d.Name,
			OriginId: d.OriginID,
		})
	}
	return resp, nil
}

func (s *adminControlServer) PublishConfig(ctx context.Context, req *controlpb.PublishConfigRequest) (*controlpb.PublishConfigResponse, error) {
	if err := requireServiceToken(ctx, s.cfg.ServiceToken); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request required")
	}

	version := strings.TrimSpace(req.GetVersion())
	nodeIDs := uniqueNonEmpty(req.GetNodeIds())

	if err := s.publisher.Publish(ctx, version, nodeIDs); err != nil {
		log.Ctx(ctx).Error().Err(err).Str("version", version).Msg("publish failed")
		return &controlpb.PublishConfigResponse{Ok: false, Message: err.Error()}, nil
	}

	log.Ctx(ctx).Info().
		Str("version", version).
		Int("node_ids", len(nodeIDs)).
		Msg("publish config completed")

	return &controlpb.PublishConfigResponse{Ok: true, Message: "publish completed"}, nil
}

func (s *adminControlServer) Purge(ctx context.Context, req *controlpb.PurgeRequest) (*controlpb.PurgeResponse, error) {
	if err := requireServiceToken(ctx, s.cfg.ServiceToken); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request required")
	}

	urls := uniqueNonEmpty(req.GetUrls())
	if len(urls) == 0 {
		return nil, status.Error(codes.InvalidArgument, "urls required")
	}

	if err := s.purge.PurgeURLs(ctx, urls); err != nil {
		return &controlpb.PurgeResponse{
			Ok:      false,
			Message: err.Error(),
		}, nil
	}
	return &controlpb.PurgeResponse{Ok: true, Message: "purge dispatched"}, nil
}

func (s *adminControlServer) Ping(ctx context.Context, req *controlpb.AdminPingRequest) (*controlpb.AdminPingResponse, error) {
	_ = req
	return &controlpb.AdminPingResponse{Ok: true, Message: "lingcdn-control admin"}, nil
}

func requireServiceToken(ctx context.Context, serviceToken string) error {
	if serviceToken == "" {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "unauthorized")
	}
	getFirst := func(key string) string {
		if vals := md.Get(key); len(vals) > 0 {
			return vals[0]
		}
		return ""
	}

	token := extractToken(getFirst("authorization"))
	if token == "" {
		token = extractToken(getFirst("x-service-token"))
	}
	if token == "" {
		return status.Error(codes.Unauthenticated, "unauthorized")
	}

	if subtle.ConstantTimeCompare([]byte(token), []byte(serviceToken)) != 1 {
		return status.Error(codes.Unauthenticated, "unauthorized")
	}
	return nil
}

func extractToken(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	if strings.EqualFold(parts[0], "bearer") {
		return parts[1]
	}
	return parts[0]
}

func normalizeDomainName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" {
		return "", fmt.Errorf("domain name required")
	}

	wildcard := false
	if strings.HasPrefix(name, "*.") {
		wildcard = true
		name = strings.TrimPrefix(name, "*.")
	}

	name = strings.TrimSuffix(name, ".")
	if name == "" {
		return "", fmt.Errorf("domain name required")
	}
	if len(name) > 253 {
		return "", fmt.Errorf("domain name too long (%d chars, max 253)", len(name))
	}
	if strings.Contains(name, "..") {
		return "", fmt.Errorf("domain name contains consecutive dots")
	}
	if strings.ContainsAny(name, " /\\\t\r\n") {
		return "", fmt.Errorf("domain name contains illegal characters (spaces, slashes, or control chars)")
	}

	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("domain name must have at least two labels (e.g. example.com), got %q", name)
	}
	for _, label := range labels {
		if !domainLabelPattern.MatchString(label) {
			return "", fmt.Errorf("invalid domain label %q: must match [a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?", label)
		}
	}

	if wildcard {
		return "*." + name, nil
	}
	return name, nil
}

func uniqueNonEmpty(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
