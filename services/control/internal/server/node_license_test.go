package server

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lingcdn/control/internal/config"
	"github.com/lingcdn/control/internal/nodehub"
	"github.com/lingcdn/control/internal/store"
	controlpb "github.com/lingcdn/control/proto/gen"
)

func TestRegisterNodeRespectsLicenseLimit(t *testing.T) {
	db := store.NewMemory("", "")
	hub := nodehub.New()
	srv := &Servers{store: db, cfg: config.Config{LicenseMode: "online"}}
	srv.setLicenseState(licenseState{Status: "active", MaxNodes: 1})

	nc := newNodeControlServer(hub, nil, nil, nil, db, nil, config.Config{}, nil, nil, srv.licenseAllowsNewNode, nil, nil, nil)

	tok, _, err := db.CreateBootstrapToken(context.Background(), "test", 60)
	if err != nil {
		t.Fatalf("create bootstrap token: %v", err)
	}

	if _, err := nc.RegisterNode(context.Background(), &controlpb.RegisterNodeRequest{Hostname: "node-1", Version: "1.0", BootstrapToken: tok}); err != nil {
		t.Fatalf("first register: %v", err)
	}

	if _, err := nc.RegisterNode(context.Background(), &controlpb.RegisterNodeRequest{Hostname: "node-2", Version: "1.0", BootstrapToken: tok}); err == nil {
		t.Fatalf("expected permission denied on second register")
	} else {
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("expected status error, got: %v", err)
		}
		if st.Code() != codes.PermissionDenied {
			t.Fatalf("expected PermissionDenied, got: %v", st.Code())
		}
	}
}

func TestUnlicensedModeBlocksNewNodeRegistration(t *testing.T) {
	db := store.NewMemory("", "")
	hub := nodehub.New()
	srv := &Servers{store: db, cfg: config.Config{LicenseMode: "online"}}

	nc := newNodeControlServer(hub, nil, nil, nil, db, nil, config.Config{}, nil, nil, srv.licenseAllowsNewNode, nil, nil, nil)

	tok, _, err := db.CreateBootstrapToken(context.Background(), "test", 60)
	if err != nil {
		t.Fatalf("create bootstrap token: %v", err)
	}

	if _, err := nc.RegisterNode(context.Background(), &controlpb.RegisterNodeRequest{Hostname: "node-open-1", Version: "1.0", BootstrapToken: tok}); err == nil {
		t.Fatalf("expected unlicensed registration to fail")
	}
}

func TestExistingNodeRegisterRequiresValidCredential(t *testing.T) {
	db := store.NewMemory("", "")
	hub := nodehub.New()
	srv := &Servers{store: db, cfg: config.Config{LicenseMode: "online"}}
	srv.setLicenseState(licenseState{Status: "active", MaxNodes: 10})

	// 创建一个 bootstrap token，这样 memory store 就不再对任意 token 返回 true
	if _, _, err := db.CreateBootstrapToken(context.Background(), "test", 60); err != nil {
		t.Fatalf("create bootstrap token: %v", err)
	}

	if err := db.CreateNode(context.Background(), &store.Node{
		ID:       "node-existing",
		Hostname: "node-existing",
		Status:   "online",
		Token:    hashToken("node-secret"),
	}); err != nil {
		t.Fatalf("create existing node: %v", err)
	}

	nc := newNodeControlServer(hub, nil, nil, nil, db, nil, config.Config{}, nil, nil, srv.licenseAllowsNewNode, nil, nil, nil)

	if _, err := nc.RegisterNode(context.Background(), &controlpb.RegisterNodeRequest{Hostname: "node-existing", Version: "1.0", BootstrapToken: "wrong-secret"}); err == nil {
		t.Fatalf("expected invalid credential for existing node")
	} else {
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("expected status error, got: %v", err)
		}
		if st.Code() != codes.PermissionDenied {
			t.Fatalf("expected PermissionDenied, got: %v", st.Code())
		}
	}
}

func TestExistingNodeRegisterAcceptsPersistedNodeToken(t *testing.T) {
	db := store.NewMemory("", "")
	hub := nodehub.New()
	srv := &Servers{store: db, cfg: config.Config{LicenseMode: "online"}}

	if err := db.CreateNode(context.Background(), &store.Node{
		ID:       "node-existing",
		Hostname: "node-existing",
		Status:   "online",
		Token:    hashToken("node-secret"),
	}); err != nil {
		t.Fatalf("create existing node: %v", err)
	}

	nc := newNodeControlServer(hub, nil, nil, nil, db, nil, config.Config{}, nil, nil, srv.licenseAllowsNewNode, nil, nil, nil)

	resp, err := nc.RegisterNode(context.Background(), &controlpb.RegisterNodeRequest{Hostname: "node-existing", Version: "1.0", BootstrapToken: "node-secret"})
	if err != nil {
		t.Fatalf("register existing node with persisted token: %v", err)
	}
	if resp.GetNodeId() != "node-existing" {
		t.Fatalf("node_id=%q want node-existing", resp.GetNodeId())
	}
	if resp.GetToken() == "" {
		t.Fatalf("expected refreshed node token")
	}
}

func TestExistingNodeCanReregisterWhenUnlicensed(t *testing.T) {
	db := store.NewMemory("", "")
	hub := nodehub.New()
	srv := &Servers{store: db, cfg: config.Config{LicenseMode: "online"}}
	// 设置 unlicensed 状态
	srv.setLicenseState(licenseState{Status: "unlicensed"})

	if err := db.CreateNode(context.Background(), &store.Node{
		ID:       "node-existing",
		Hostname: "node-existing",
		Status:   "online",
		Token:    hashToken("node-secret"),
	}); err != nil {
		t.Fatalf("create existing node: %v", err)
	}

	nc := newNodeControlServer(hub, nil, nil, nil, db, nil, config.Config{}, nil, nil, srv.licenseAllowsNewNode, nil, nil, nil)

	// 已有节点在 unlicensed 状态下应能重注册
	resp, err := nc.RegisterNode(context.Background(), &controlpb.RegisterNodeRequest{Hostname: "node-existing", Version: "1.0", BootstrapToken: "node-secret"})
	if err != nil {
		t.Fatalf("existing node re-register under unlicensed should succeed, got: %v", err)
	}
	if resp.GetNodeId() != "node-existing" {
		t.Fatalf("node_id=%q want node-existing", resp.GetNodeId())
	}
}

func TestNewNodeBlockedWhenUnlicensed(t *testing.T) {
	db := store.NewMemory("", "")
	hub := nodehub.New()
	srv := &Servers{store: db, cfg: config.Config{LicenseMode: "online"}}
	srv.setLicenseState(licenseState{Status: "unlicensed"})

	nc := newNodeControlServer(hub, nil, nil, nil, db, nil, config.Config{}, nil, nil, srv.licenseAllowsNewNode, nil, nil, nil)

	tok, _, err := db.CreateBootstrapToken(context.Background(), "test", 60)
	if err != nil {
		t.Fatalf("create bootstrap token: %v", err)
	}

	// 全新节点在 unlicensed 状态下应被拒绝
	_, err = nc.RegisterNode(context.Background(), &controlpb.RegisterNodeRequest{Hostname: "brand-new-node", Version: "1.0", BootstrapToken: tok})
	if err == nil {
		t.Fatalf("expected new node registration to fail under unlicensed")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got: %v", err)
	}
	if st.Code() != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got: %v", st.Code())
	}
}

func TestValidateNodeTokenRejectsEmptyToken(t *testing.T) {
	db := store.NewMemory("", "")

	if err := db.CreateNode(context.Background(), &store.Node{
		ID:       "node-1",
		Hostname: "node-1",
		Status:   "online",
		Token:    hashToken("real-token"),
	}); err != nil {
		t.Fatalf("create node: %v", err)
	}

	nc := newNodeControlServer(nodehub.New(), nil, nil, nil, db, nil, config.Config{}, nil, nil, nil, nil, nil, nil)

	// 空 token 应被拒绝
	err := nc.validateNodeToken(context.Background(), "node-1", "")
	if err == nil {
		t.Fatalf("expected empty token to be rejected")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got: %v", err)
	}
	if st.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got: %v", st.Code())
	}
}

func TestValidateNodeTokenRejectsNodeWithNoStoredToken(t *testing.T) {
	db := store.NewMemory("", "")

	// 模拟历史遗留节点：数据库中 Token 为空
	if err := db.CreateNode(context.Background(), &store.Node{
		ID:       "legacy-node",
		Hostname: "legacy-node",
		Status:   "online",
		Token:    "", // 无存储 token
	}); err != nil {
		t.Fatalf("create node: %v", err)
	}

	nc := newNodeControlServer(nodehub.New(), nil, nil, nil, db, nil, config.Config{}, nil, nil, nil, nil, nil, nil)

	// 即使提供了 token，节点库里无 token 也应被拒绝
	err := nc.validateNodeToken(context.Background(), "legacy-node", "any-token")
	if err == nil {
		t.Fatalf("expected node with no stored token to be rejected")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got: %v", err)
	}
	if st.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got: %v", st.Code())
	}
}
