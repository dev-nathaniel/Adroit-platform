package userservice_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
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
	inMemStore := userservice.NewInMemStorage("/api/v1/users/avatars")
	svc := userservice.NewService(repo, userservice.WithStorageProvider(inMemStore))
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

func TestAvatarUploadAndServeLifecycle(t *testing.T) {
	r, repo, jwtSecret, alice, _ := setupUserTestRouter()
	aliceToken, _ := identity.GenerateJWT(alice.ID, alice.Email, jwtSecret, 1*time.Hour)

	// Valid PNG payload (8-byte PNG header + chunk data)
	pngData := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15c4")

	// 1. Upload Avatar
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("avatar", "profile.png")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write(pngData); err != nil {
		t.Fatalf("failed to write png data: %v", err)
	}
	_ = writer.Close()

	reqUpload := httptest.NewRequest(http.MethodPost, "/profile/avatar", body)
	reqUpload.Header.Set("Authorization", "Bearer "+aliceToken)
	reqUpload.Header.Set("Content-Type", writer.FormDataContentType())
	recUpload := httptest.NewRecorder()
	r.ServeHTTP(recUpload, reqUpload)

	if recUpload.Code != http.StatusOK {
		t.Fatalf("expected 200 on avatar upload, got %d. Body: %s", recUpload.Code, recUpload.Body.String())
	}

	var res response.APIResponse[userservice.AvatarUploadResponse]
	if err := json.Unmarshal(recUpload.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse avatar upload response: %v", err)
	}

	if !strings.Contains(res.Data.AvatarURL, "avatar_usr-alice-111_") {
		t.Errorf("expected avatar URL to contain user ID, got %s", res.Data.AvatarURL)
	}

	// Verify user record in repository
	updatedUser, _ := repo.GetUser(context.Background(), alice.ID)
	if updatedUser.AvatarURL != res.Data.AvatarURL {
		t.Errorf("expected repo avatar URL %s, got %s", res.Data.AvatarURL, updatedUser.AvatarURL)
	}

	// 2. Fetch Avatar via Public Endpoint
	// res.Data.AvatarURL is e.g. "/api/v1/users/avatars/avatar_usr-alice-111_..."
	// The route registered in chi is "/avatars/{filename}"
	parts := strings.Split(res.Data.AvatarURL, "/avatars/")
	if len(parts) != 2 {
		t.Fatalf("invalid avatar URL format: %s", res.Data.AvatarURL)
	}
	filename := parts[1]

	reqGet := httptest.NewRequest(http.MethodGet, "/avatars/"+filename, nil)
	recGet := httptest.NewRecorder()
	r.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 on get avatar, got %d", recGet.Code)
	}
	if recGet.Header().Get("Content-Type") != "image/png" {
		t.Errorf("expected Content-Type image/png, got %s", recGet.Header().Get("Content-Type"))
	}
	if !bytes.Equal(recGet.Body.Bytes(), pngData) {
		t.Errorf("served avatar data does not match uploaded content")
	}

	// 3. Delete Avatar
	reqDelete := httptest.NewRequest(http.MethodDelete, "/profile/avatar", nil)
	reqDelete.Header.Set("Authorization", "Bearer "+aliceToken)
	recDelete := httptest.NewRecorder()
	r.ServeHTTP(recDelete, reqDelete)

	if recDelete.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete avatar, got %d", recDelete.Code)
	}

	clearedUser, _ := repo.GetUser(context.Background(), alice.ID)
	if clearedUser.AvatarURL != "" {
		t.Errorf("expected avatar URL to be cleared, got %s", clearedUser.AvatarURL)
	}
}

func TestAvatarUploadValidation(t *testing.T) {
	r, _, jwtSecret, alice, _ := setupUserTestRouter()
	aliceToken, _ := identity.GenerateJWT(alice.ID, alice.Email, jwtSecret, 1*time.Hour)

	t.Run("Rejects non-image file", func(t *testing.T) {
		body := new(bytes.Buffer)
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("avatar", "test.txt")
		_, _ = part.Write([]byte("this is plain text not an image"))
		_ = writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/profile/avatar", body)
		req.Header.Set("Authorization", "Bearer "+aliceToken)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for text file, got %d", rec.Code)
		}
	})

	t.Run("Requires authentication", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/profile/avatar", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 without token, got %d", rec.Code)
		}
	})

	t.Run("Missing avatar field", func(t *testing.T) {
		body := new(bytes.Buffer)
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("other", "value")
		_ = writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/profile/avatar", body)
		req.Header.Set("Authorization", "Bearer "+aliceToken)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing field, got %d", rec.Code)
		}
	})
}


