package userservice_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	userservice "github.com/dev-nathaniel/Adroit-platform/internal/user-service"
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
