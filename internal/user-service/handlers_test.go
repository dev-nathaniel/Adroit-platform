package userservice_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	userservice "github.com/dev-nathaniel/Adroit-platform/internal/user-service"
	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/identity"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

func setupUserTestRouter() (chi.Router, *userservice.MemoryRepository, string, *types.User, *types.User) {
	jwtSecret := "super-secure-testing-secret-key-32b"
	repo := userservice.NewMemoryRepository()
	svc := userservice.NewService(repo)
	handler := userservice.NewHandler(svc)

	user1 := &types.User{
		ID:        "usr-alice-111",
		Email:     "alice@adroit.run",
		Name:      "Alice Engineer",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	user2 := &types.User{
		ID:        "usr-bob-222",
		Email:     "bob@adroit.run",
		Name:      "Bob DevOps",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	repo.SeedUser(user1)
	repo.SeedUser(user2)

	r := chi.NewRouter()
	handler.RegisterRoutes(r, jwtSecret)

	return r, repo, jwtSecret, user1, user2
}

func TestUserProfileAndOrgFlow(t *testing.T) {
	r, _, jwtSecret, user1, user2 := setupUserTestRouter()

	aliceToken, _ := identity.GenerateJWT(user1.ID, user1.Email, jwtSecret, 1*time.Hour)
	bobToken, _ := identity.GenerateJWT(user2.ID, user2.Email, jwtSecret, 1*time.Hour)

	// 1. Get Profile
	reqProfile := httptest.NewRequest(http.MethodGet, "/profile", nil)
	reqProfile.Header.Set("Authorization", "Bearer "+aliceToken)
	recProfile := httptest.NewRecorder()
	r.ServeHTTP(recProfile, reqProfile)

	if recProfile.Code != http.StatusOK {
		t.Fatalf("expected 200 on profile, got %d", recProfile.Code)
	}

	// 2. Update Profile
	updateBody, _ := json.Marshal(userservice.UpdateProfileRequest{Name: "Alice Senior Architect"})
	reqUpdate := httptest.NewRequest(http.MethodPatch, "/profile", bytes.NewReader(updateBody))
	reqUpdate.Header.Set("Authorization", "Bearer "+aliceToken)
	recUpdate := httptest.NewRecorder()
	r.ServeHTTP(recUpdate, reqUpdate)

	if recUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 on update profile, got %d", recUpdate.Code)
	}

	// 3. Create Organization
	createOrgBody, _ := json.Marshal(userservice.CreateOrgRequest{Name: "Acme Cloud"})
	reqCreateOrg := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewReader(createOrgBody))
	reqCreateOrg.Header.Set("Authorization", "Bearer "+aliceToken)
	recCreateOrg := httptest.NewRecorder()
	r.ServeHTTP(recCreateOrg, reqCreateOrg)

	if recCreateOrg.Code != http.StatusCreated {
		t.Fatalf("expected 201 on create org, got %d", recCreateOrg.Code)
	}

	var orgRes response.APIResponse[types.Organization]
	_ = json.Unmarshal(recCreateOrg.Body.Bytes(), &orgRes)
	orgID := orgRes.Data.ID

	// 4. List Organizations for Alice
	reqListOrgs := httptest.NewRequest(http.MethodGet, "/organizations", nil)
	reqListOrgs.Header.Set("Authorization", "Bearer "+aliceToken)
	recListOrgs := httptest.NewRecorder()
	r.ServeHTTP(recListOrgs, reqListOrgs)

	if recListOrgs.Code != http.StatusOK {
		t.Fatalf("expected 200 on list orgs, got %d", recListOrgs.Code)
	}

	// 5. Add Bob as member
	addMemberBody, _ := json.Marshal(userservice.AddMemberRequest{
		Email: "bob@adroit.run",
		Role:  "admin",
	})
	reqAddMember := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID+"/members", bytes.NewReader(addMemberBody))
	reqAddMember.Header.Set("Authorization", "Bearer "+aliceToken)
	recAddMember := httptest.NewRecorder()
	r.ServeHTTP(recAddMember, reqAddMember)

	if recAddMember.Code != http.StatusCreated {
		t.Fatalf("expected 201 on add member, got %d. Body: %s", recAddMember.Code, recAddMember.Body.String())
	}

	// 6. List Members
	reqListMembers := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID+"/members", nil)
	reqListMembers.Header.Set("Authorization", "Bearer "+bobToken)
	recListMembers := httptest.NewRecorder()
	r.ServeHTTP(recListMembers, reqListMembers)

	if recListMembers.Code != http.StatusOK {
		t.Fatalf("expected 200 on list members, got %d", recListMembers.Code)
	}

	var membersRes response.APIResponse[[]userservice.OrgMemberInfo]
	_ = json.Unmarshal(recListMembers.Body.Bytes(), &membersRes)
	if len(membersRes.Data) != 2 {
		t.Errorf("expected 2 members, got %d", len(membersRes.Data))
	}
}

func TestInvitationLifecycleFlow(t *testing.T) {
	r, repo, jwtSecret, alice, bob := setupUserTestRouter()

	aliceToken, _ := identity.GenerateJWT(alice.ID, alice.Email, jwtSecret, 1*time.Hour)
	bobToken, _ := identity.GenerateJWT(bob.ID, bob.Email, jwtSecret, 1*time.Hour)

	// 1. Alice creates workspace
	createOrgBody, _ := json.Marshal(userservice.CreateOrgRequest{Name: "Acme Workspace"})
	reqCreate := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewReader(createOrgBody))
	reqCreate.Header.Set("Authorization", "Bearer "+aliceToken)
	recCreate := httptest.NewRecorder()
	r.ServeHTTP(recCreate, reqCreate)

	var orgRes response.APIResponse[types.Organization]
	_ = json.Unmarshal(recCreate.Body.Bytes(), &orgRes)
	orgID := orgRes.Data.ID

	// 2. Alice invites unregistered user Charlie
	inviteReqBody, _ := json.Marshal(userservice.CreateInvitationRequest{
		Email: "charlie@adroit.run",
		Role:  "admin",
	})
	reqInvite := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID+"/invitations", bytes.NewReader(inviteReqBody))
	reqInvite.Header.Set("Authorization", "Bearer "+aliceToken)
	recInvite := httptest.NewRecorder()
	r.ServeHTTP(recInvite, reqInvite)

	if recInvite.Code != http.StatusCreated {
		t.Fatalf("expected 201 on invite, got %d: %s", recInvite.Code, recInvite.Body.String())
	}

	var inviteRes response.APIResponse[userservice.InvitationResponse]
	_ = json.Unmarshal(recInvite.Body.Bytes(), &inviteRes)
	token := inviteRes.Data.Token
	if token == "" {
		t.Fatal("expected non-empty token in invitation response")
	}

	// 3. Duplicate invite returns 409 Conflict
	reqDupInvite := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID+"/invitations", bytes.NewReader(inviteReqBody))
	reqDupInvite.Header.Set("Authorization", "Bearer "+aliceToken)
	recDupInvite := httptest.NewRecorder()
	r.ServeHTTP(recDupInvite, reqDupInvite)
	if recDupInvite.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate invite, got %d", recDupInvite.Code)
	}

	// 4. Bob (non-member) tries to invite -> 403 Forbidden
	reqBobInvite := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID+"/invitations", bytes.NewReader(inviteReqBody))
	reqBobInvite.Header.Set("Authorization", "Bearer "+bobToken)
	recBobInvite := httptest.NewRecorder()
	r.ServeHTTP(recBobInvite, reqBobInvite)
	if recBobInvite.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when non-member invites, got %d", recBobInvite.Code)
	}

	// 5. Public preview (unauthenticated)
	reqPreview := httptest.NewRequest(http.MethodGet, "/invitations/"+token, nil)
	recPreview := httptest.NewRecorder()
	r.ServeHTTP(recPreview, reqPreview)

	if recPreview.Code != http.StatusOK {
		t.Fatalf("expected 200 on public preview, got %d: %s", recPreview.Code, recPreview.Body.String())
	}

	var previewRes response.APIResponse[userservice.InvitationPreview]
	_ = json.Unmarshal(recPreview.Body.Bytes(), &previewRes)
	if previewRes.Data.Email != "charlie@adroit.run" || previewRes.Data.OrganizationName != "Acme Workspace" {
		t.Fatalf("unexpected preview data: %+v", previewRes.Data)
	}

	// 6. Bob tries to accept Charlie's invitation -> 403 Forbidden (email mismatch)
	reqBobAccept := httptest.NewRequest(http.MethodPost, "/invitations/"+token+"/accept", nil)
	reqBobAccept.Header.Set("Authorization", "Bearer "+bobToken)
	recBobAccept := httptest.NewRecorder()
	r.ServeHTTP(recBobAccept, reqBobAccept)
	if recBobAccept.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on email mismatch accept, got %d", recBobAccept.Code)
	}

	// 7. Charlie registers account and accepts
	charlie := &types.User{
		ID:        "usr-charlie-333",
		Email:     "charlie@adroit.run",
		Name:      "Charlie Cloud",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	repo.SeedUser(charlie)
	charlieToken, _ := identity.GenerateJWT(charlie.ID, charlie.Email, jwtSecret, 1*time.Hour)

	reqCharlieAccept := httptest.NewRequest(http.MethodPost, "/invitations/"+token+"/accept", nil)
	reqCharlieAccept.Header.Set("Authorization", "Bearer "+charlieToken)
	recCharlieAccept := httptest.NewRecorder()
	r.ServeHTTP(recCharlieAccept, reqCharlieAccept)

	if recCharlieAccept.Code != http.StatusOK {
		t.Fatalf("expected 200 on Charlie accept, got %d: %s", recCharlieAccept.Code, recCharlieAccept.Body.String())
	}

	// 8. Verify Charlie is now in org members
	reqMembers := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID+"/members", nil)
	reqMembers.Header.Set("Authorization", "Bearer "+charlieToken)
	recMembers := httptest.NewRecorder()
	r.ServeHTTP(recMembers, reqMembers)

	var membersRes response.APIResponse[[]userservice.OrgMemberInfo]
	_ = json.Unmarshal(recMembers.Body.Bytes(), &membersRes)
	foundCharlie := false
	for _, m := range membersRes.Data {
		if m.UserID == charlie.ID && m.Role == "admin" {
			foundCharlie = true
			break
		}
	}
	if !foundCharlie {
		t.Fatal("expected Charlie to be in organization members as admin")
	}

	// 9. Re-accepting the same token returns 404 (already used)
	reqReaccept := httptest.NewRequest(http.MethodPost, "/invitations/"+token+"/accept", nil)
	reqReaccept.Header.Set("Authorization", "Bearer "+charlieToken)
	recReaccept := httptest.NewRecorder()
	r.ServeHTTP(recReaccept, reqReaccept)
	if recReaccept.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on reused token, got %d", recReaccept.Code)
	}

	// 10. Invite Revocation Flow
	inviteDaveBody, _ := json.Marshal(userservice.CreateInvitationRequest{
		Email: "dave@adroit.run",
		Role:  "member",
	})
	reqInviteDave := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID+"/invitations", bytes.NewReader(inviteDaveBody))
	reqInviteDave.Header.Set("Authorization", "Bearer "+aliceToken)
	recInviteDave := httptest.NewRecorder()
	r.ServeHTTP(recInviteDave, reqInviteDave)

	var daveRes response.APIResponse[userservice.InvitationResponse]
	_ = json.Unmarshal(recInviteDave.Body.Bytes(), &daveRes)
	daveInviteID := daveRes.Data.ID
	daveToken := daveRes.Data.Token

	// List pending invitations
	reqListInv := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID+"/invitations", nil)
	reqListInv.Header.Set("Authorization", "Bearer "+aliceToken)
	recListInv := httptest.NewRecorder()
	r.ServeHTTP(recListInv, reqListInv)

	var listRes response.APIResponse[[]types.OrganizationInvitation]
	_ = json.Unmarshal(recListInv.Body.Bytes(), &listRes)
	if len(listRes.Data) < 1 {
		t.Fatalf("expected at least 1 pending invite, got %d", len(listRes.Data))
	}

	// Alice revokes Dave's invitation
	reqRevoke := httptest.NewRequest(http.MethodDelete, "/organizations/"+orgID+"/invitations/"+daveInviteID, nil)
	reqRevoke.Header.Set("Authorization", "Bearer "+aliceToken)
	recRevoke := httptest.NewRecorder()
	r.ServeHTTP(recRevoke, reqRevoke)
	if recRevoke.Code != http.StatusOK {
		t.Fatalf("expected 200 on revoke invite, got %d", recRevoke.Code)
	}

	// Previewing revoked token fails
	reqDavePreview := httptest.NewRequest(http.MethodGet, "/invitations/"+daveToken, nil)
	recDavePreview := httptest.NewRecorder()
	r.ServeHTTP(recDavePreview, reqDavePreview)
	if recDavePreview.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on revoked token preview, got %d", recDavePreview.Code)
	}
}

func TestInvitationEventEmission(t *testing.T) {
	jwtSecret := "super-secure-testing-secret-key-32b"
	repo := userservice.NewMemoryRepository()
	bus := events.NewInMemBus()
	defer bus.Close()

	svc := userservice.NewService(repo, userservice.WithPublisher(bus))
	handler := userservice.NewHandler(svc)

	user1 := &types.User{
		ID:        "usr-alice-111",
		Email:     "alice@adroit.run",
		Name:      "Alice Engineer",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	repo.SeedUser(user1)

	r := chi.NewRouter()
	handler.RegisterRoutes(r, jwtSecret)

	aliceToken, _ := identity.GenerateJWT(user1.ID, user1.Email, jwtSecret, 1*time.Hour)

	// Create org
	createOrgBody, _ := json.Marshal(userservice.CreateOrgRequest{Name: "Event Corp"})
	reqCreate := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewReader(createOrgBody))
	reqCreate.Header.Set("Authorization", "Bearer "+aliceToken)
	recCreate := httptest.NewRecorder()
	r.ServeHTTP(recCreate, reqCreate)

	var orgRes response.APIResponse[types.Organization]
	_ = json.Unmarshal(recCreate.Body.Bytes(), &orgRes)
	orgID := orgRes.Data.ID

	// Listen for event
	var wg sync.WaitGroup
	wg.Add(1)
	var capturedEmail string
	var capturedOrg string
	_ = bus.Subscribe(events.SubjectOrgMemberInvited, func(ctx context.Context, subject string, rawData []byte) error {
		defer wg.Done()
		var p events.OrgMemberInvitedPayload
		_ = json.Unmarshal(rawData, &p)
		capturedEmail = p.Email
		capturedOrg = p.OrganizationName
		return nil
	})

	// Invite user
	inviteBody, _ := json.Marshal(userservice.CreateInvitationRequest{
		Email: "newhire@eventcorp.io",
		Role:  "member",
	})
	reqInvite := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID+"/invitations", bytes.NewReader(inviteBody))
	reqInvite.Header.Set("Authorization", "Bearer "+aliceToken)
	recInvite := httptest.NewRecorder()
	r.ServeHTTP(recInvite, reqInvite)

	if recInvite.Code != http.StatusCreated {
		t.Fatalf("expected 201 on invite, got %d", recInvite.Code)
	}

	wg.Wait()

	if capturedEmail != "newhire@eventcorp.io" {
		t.Errorf("expected captured email newhire@eventcorp.io, got %s", capturedEmail)
	}
	if capturedOrg != "Event Corp" {
		t.Errorf("expected captured org Event Corp, got %s", capturedOrg)
	}
}


