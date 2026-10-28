package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
	"github.com/hutaomu/hutaomuchat/server/internal/models"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
)

// handleListMoments returns the public user circle.
func (s *Server) handleListMoments(w http.ResponseWriter, r *http.Request) {
	s.writeMoments(w, r, s.Store.PublicMoments())
}

// handleFriendMoments returns the friend feed of the authenticated user.
func (s *Server) handleFriendMoments(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	s.writeMoments(w, r, s.Store.FriendMoments(user.ID))
}

func (s *Server) writeMoments(w http.ResponseWriter, r *http.Request, moments []*models.Moment) {
	user := currentUser(r)
	out := make([]models.MomentDTO, 0, len(moments))
	for _, moment := range moments {
		out = append(out, s.momentDTO(moment, user.ID))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"moments": out})
}

// handleCreateMoment publishes a new moment.
func (s *Server) handleCreateMoment(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		Content  string          `json:"content"`
		Images   json.RawMessage `json:"images"`
		Scope    string          `json:"scope"`
		Location string          `json:"location"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	content := strings.TrimSpace(req.Content)
	images := parseImageList(req.Images)
	if content == "" && len(images) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "动态内容不能为空")
		return
	}
	scope := strings.ToLower(strings.TrimSpace(req.Scope))
	if scope != models.ScopeFriends {
		scope = models.ScopePublic
	}

	moment := s.Store.AddMoment(&models.Moment{
		ID:        store.NewID(),
		UserID:    user.ID,
		Content:   truncate(content, 2000),
		Images:    images,
		Scope:     scope,
		Location:  truncate(strings.TrimSpace(req.Location), 64),
		CreatedAt: models.Now(),
	})
	s.Logger.User(user.ID, "POST_MOMENT", map[string]any{"momentId": moment.ID, "scope": scope})

	// Notify the friends of the author about the new moment.
	for _, friendID := range s.Store.FriendIDs(user.ID) {
		s.Hub.SendToUser(friendID, map[string]any{
			"type":     "moment_new",
			"momentId": moment.ID,
			"authorId": user.ID,
		})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"moment": s.momentDTO(moment, user.ID)})
}

// handleLikeMoment toggles the like state of a moment.
func (s *Server) handleLikeMoment(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	moment := s.Store.FindMoment(r.PathValue("id"))
	if moment == nil {
		httpx.WriteError(w, http.StatusNotFound, "动态不存在")
		return
	}
	liked := s.Store.LikeMoment(moment.ID, user.ID)
	if liked {
		s.Hub.SendToUser(moment.UserID, map[string]any{
			"type":     "moment_like",
			"momentId": moment.ID,
			"userId":   user.ID,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "liked": liked})
}

// handleUnlikeMoment removes the like of a moment.
func (s *Server) handleUnlikeMoment(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	moment := s.Store.FindMoment(r.PathValue("id"))
	if moment == nil {
		httpx.WriteError(w, http.StatusNotFound, "动态不存在")
		return
	}
	s.Store.LikeMoment(moment.ID, user.ID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "liked": false})
}

// handleCommentMoment adds a comment to a moment.
func (s *Server) handleCommentMoment(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		Content  string `json:"content"`
		ImageURL string `json:"image_url"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	moment := s.Store.FindMoment(r.PathValue("id"))
	if moment == nil {
		httpx.WriteError(w, http.StatusNotFound, "动态不存在")
		return
	}
	content := strings.TrimSpace(req.Content)
	imageURL := strings.TrimSpace(req.ImageURL)
	if content == "" && imageURL == "" {
		httpx.WriteError(w, http.StatusBadRequest, "评论内容不能为空")
		return
	}
	comment := s.Store.AddMomentComment(&models.MomentComment{
		ID:        store.NewID(),
		MomentID:  moment.ID,
		UserID:    user.ID,
		Content:   truncate(content, 500),
		ImageURL:  truncate(imageURL, 512),
		Nickname:  user.Nickname,
		CreatedAt: models.Now(),
	})
	s.Hub.SendToUser(moment.UserID, map[string]any{
		"type":     "moment_comment",
		"momentId": moment.ID,
		"comment":  comment,
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"comment": comment})
}

// handleDeleteMoment removes a moment owned by the authenticated user.
func (s *Server) handleDeleteMoment(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	moment := s.Store.FindMoment(r.PathValue("id"))
	if moment == nil {
		httpx.WriteError(w, http.StatusNotFound, "动态不存在")
		return
	}
	if moment.UserID != user.ID {
		httpx.WriteError(w, http.StatusForbidden, "只能删除自己的动态")
		return
	}
	s.Store.RemoveMoment(moment.ID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// momentDTO enriches a moment with the author, likes and comments.
func (s *Server) momentDTO(moment *models.Moment, viewerID string) models.MomentDTO {
	dto := models.MomentDTO{
		ID:        moment.ID,
		UserID:    moment.UserID,
		Content:   moment.Content,
		Images:    models.ImageList(moment.Images),
		Scope:     moment.Scope,
		Location:  moment.Location,
		CreatedAt: moment.CreatedAt,
		Likes:     []models.UserDTO{},
		Comments:  []models.MomentCommentDTO{},
		Liked:     s.Store.IsLiked(moment.ID, viewerID),
	}
	if author := s.Store.FindUserByID(moment.UserID); author != nil {
		profile := models.NewUserDTO(author)
		dto.User = &profile
	}
	for _, likerID := range s.Store.MomentLikes(moment.ID) {
		if liker := s.Store.FindUserByID(likerID); liker != nil {
			dto.Likes = append(dto.Likes, models.NewUserDTO(liker))
		}
	}
	for _, comment := range s.Store.MomentComments(moment.ID) {
		dto.Comments = append(dto.Comments, models.MomentCommentDTO{
			ID:        comment.ID,
			MomentID:  comment.MomentID,
			UserID:    comment.UserID,
			Content:   comment.Content,
			ImageURL:  comment.ImageURL,
			Nickname:  comment.Nickname,
			CreatedAt: comment.CreatedAt,
		})
	}
	return dto
}
