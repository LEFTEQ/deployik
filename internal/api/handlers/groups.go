package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/lefteq/lovinka-deployik/internal/auth"
	"github.com/lefteq/lovinka-deployik/internal/db"
)

type GroupHandler struct {
	DB *db.DB
}

type createGroupRequest struct {
	Name       string   `json:"name"`
	ProjectIDs []string `json:"project_ids"`
}

type updateGroupRequest struct {
	Name string `json:"name"`
}

type moveGroupProjectsRequest struct {
	ProjectIDs []string `json:"project_ids"`
}

type createGroupInviteRequest struct {
	GithubUsername string `json:"github_username"`
	Role           string `json:"role"`
}

type updateGroupMemberRequest struct {
	Role string `json:"role"`
}

func (h *GroupHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	groups, err := h.DB.ListGroupsForUser(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list groups"})
		return
	}
	if groups == nil {
		groups = []db.Group{}
	}
	writeJSON(w, http.StatusOK, groups)
}

func (h *GroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	var req createGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if !h.canAccessProjects(claims, req.ProjectIDs) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	group, err := h.DB.CreateGroup(&db.GroupCreate{
		Name:       req.Name,
		OwnerID:    claims.UserID,
		ProjectIDs: req.ProjectIDs,
	})
	if err != nil {
		if errors.Is(err, db.ErrProjectNotMovable) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create group"})
		return
	}
	writeJSON(w, http.StatusCreated, group)
}

func (h *GroupHandler) Update(w http.ResponseWriter, r *http.Request) {
	group, _, ok := h.loadManagedGroup(w, r)
	if !ok {
		return
	}
	var req updateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	updated, err := h.DB.UpdateGroupName(group.ID, req.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update group"})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *GroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	group, _, ok := h.loadManagedGroup(w, r)
	if !ok {
		return
	}
	defaultGroup, err := h.DB.ResolveDefaultGroupForGroup(group.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load default group"})
		return
	}
	if err := h.DB.DeleteGroupMovingProjects(group.ID, defaultGroup.ID); err != nil {
		if errors.Is(err, db.ErrDefaultGroupCannotBeDeleted) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "default group cannot be deleted"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete group"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *GroupHandler) MoveProjects(w http.ResponseWriter, r *http.Request) {
	group, claims, ok := h.loadManagedGroup(w, r)
	if !ok {
		return
	}
	var req moveGroupProjectsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if !h.canAccessProjects(claims, req.ProjectIDs) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	if err := h.DB.MoveProjectsToGroup(group.ID, req.ProjectIDs); err != nil {
		if errors.Is(err, db.ErrProjectNotMovable) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to move projects"})
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (h *GroupHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	group, _, ok := h.loadAccessibleGroup(w, r)
	if !ok {
		return
	}
	members, err := h.DB.ListGroupMembers(group.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list members"})
		return
	}
	if members == nil {
		members = []db.GroupMember{}
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *GroupHandler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	group, _, ok := h.loadManagedGroup(w, r)
	if !ok {
		return
	}
	userID := chi.URLParam(r, "uid")
	var req updateGroupMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.DB.UpdateGroupMemberRole(group.ID, userID, req.Role); err != nil {
		if errors.Is(err, db.ErrLastGroupOwner) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "group must keep at least one owner"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update member"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *GroupHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	group, _, ok := h.loadManagedGroup(w, r)
	if !ok {
		return
	}
	userID := chi.URLParam(r, "uid")
	if err := h.DB.RemoveGroupMember(group.ID, userID); err != nil {
		if errors.Is(err, db.ErrLastGroupOwner) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "group must keep at least one owner"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove member"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *GroupHandler) ListInvites(w http.ResponseWriter, r *http.Request) {
	group, _, ok := h.loadManagedGroup(w, r)
	if !ok {
		return
	}
	invites, err := h.DB.ListGroupInvites(group.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list invites"})
		return
	}
	if invites == nil {
		invites = []db.GroupInvite{}
	}
	writeJSON(w, http.StatusOK, invites)
}

func (h *GroupHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	group, claims, ok := h.loadManagedGroup(w, r)
	if !ok {
		return
	}
	var req createGroupInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.GithubUsername) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "github_username is required"})
		return
	}
	invite, err := h.DB.CreateGroupInvite(&db.GroupInviteCreate{
		GroupID:         group.ID,
		GithubUsername:  req.GithubUsername,
		Role:            req.Role,
		InvitedByUserID: claims.UserID,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create invite"})
		return
	}
	writeJSON(w, http.StatusCreated, invite)
}

func (h *GroupHandler) CancelInvite(w http.ResponseWriter, r *http.Request) {
	group, _, ok := h.loadManagedGroup(w, r)
	if !ok {
		return
	}
	if err := h.DB.CancelGroupInvite(group.ID, chi.URLParam(r, "iid")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to cancel invite"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *GroupHandler) ListMyInvites(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	user, err := h.DB.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
		return
	}
	invites, err := h.DB.ListPendingGroupInvitesForGithubUsername(user.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list invites"})
		return
	}
	if invites == nil {
		invites = []db.GroupInvite{}
	}
	writeJSON(w, http.StatusOK, invites)
}

func (h *GroupHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	user, err := h.DB.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
		return
	}
	if err := h.DB.AcceptGroupInvite(chi.URLParam(r, "iid"), user.ID, user.Username); err != nil {
		if errors.Is(err, db.ErrGroupInviteNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to accept invite"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *GroupHandler) DeclineInvite(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	user, err := h.DB.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
		return
	}
	if err := h.DB.DeclineGroupInvite(chi.URLParam(r, "iid"), user.Username); err != nil {
		if errors.Is(err, db.ErrGroupInviteNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decline invite"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *GroupHandler) loadManagedGroup(w http.ResponseWriter, r *http.Request) (*db.Group, *auth.Claims, bool) {
	claims := auth.GetClaims(r.Context())
	groupID := chi.URLParam(r, "id")
	if claims.Role == "admin" {
		group, err := h.DB.GetGroup(groupID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load group"})
			return nil, nil, false
		}
		if group == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
			return nil, nil, false
		}
		return group, claims, true
	}

	group, err := h.DB.GetGroupForUser(groupID, claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load group"})
		return nil, nil, false
	}
	if group == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
		return nil, nil, false
	}
	if group.MembershipRole != db.OrganizationRoleOwner {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "group owner access required"})
		return nil, nil, false
	}
	return group, claims, true
}

func (h *GroupHandler) loadAccessibleGroup(w http.ResponseWriter, r *http.Request) (*db.Group, *auth.Claims, bool) {
	claims := auth.GetClaims(r.Context())
	groupID := chi.URLParam(r, "id")
	if claims.Role == "admin" {
		group, err := h.DB.GetGroup(groupID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load group"})
			return nil, nil, false
		}
		if group == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
			return nil, nil, false
		}
		return group, claims, true
	}

	group, err := h.DB.GetGroupForUser(groupID, claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load group"})
		return nil, nil, false
	}
	if group == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
		return nil, nil, false
	}
	return group, claims, true
}

func (h *GroupHandler) canAccessProjects(claims *auth.Claims, projectIDs []string) bool {
	for _, projectID := range projectIDs {
		projectID = strings.TrimSpace(projectID)
		if projectID == "" {
			continue
		}
		var project *db.Project
		var err error
		if claims.Role == "admin" {
			project, err = h.DB.GetProject(projectID)
		} else {
			project, err = h.DB.GetProjectForUser(projectID, claims.UserID)
		}
		if err != nil || project == nil {
			return false
		}
	}
	return true
}
