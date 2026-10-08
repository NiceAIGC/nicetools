package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"nicetools/backend/internal/model"
	"nicetools/backend/internal/store"
)

type Service struct {
	S         *store.Store
	dummyHash []byte
}

func New(s *store.Store) *Service {
	hash, err := bcrypt.GenerateFromPassword([]byte("invalid-login-timing-only"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return &Service{S: s, dummyHash: hash}
}

type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string                 { return e.Message }
func problem(status int, message string) error { return &Error{Status: status, Message: message} }
func dbError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return problem(404, "记录不存在")
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return problem(409, "名称或用户名已存在")
	}
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		return problem(400, "关联的用户组或工具不存在")
	}
	return err
}
func hash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (s *Service) Issue(userID *uint, previous string) (string, string, error) {
	token, err := randomToken()
	if err != nil {
		return "", "", err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", "", err
	}
	err = s.S.DB.Transaction(func(tx *gorm.DB) error {
		if previous != "" {
			if err := tx.Where("token_hash = ?", hash(previous)).Delete(&model.Session{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("expires_at <= ?", time.Now()).Delete(&model.Session{}).Error; err != nil {
			return err
		}
		return tx.Create(&model.Session{TokenHash: hash(token), UserID: userID, CSRFHash: hash(csrf), ExpiresAt: time.Now().Add(7 * 24 * time.Hour)}).Error
	})
	return token, csrf, err
}
func (s *Service) Guest() (string, string, error) { return s.Issue(nil, "") }
func (s *Service) Session(token string) (*model.User, string, error) {
	if len(token) != 64 {
		return nil, "", gorm.ErrRecordNotFound
	}
	var session model.Session
	if err := s.S.DB.Where("token_hash = ? AND expires_at > ?", hash(token), time.Now()).First(&session).Error; err != nil {
		return nil, "", err
	}
	if session.UserID == nil {
		return nil, session.CSRFHash, nil
	}
	var user model.User
	if err := s.S.DB.Preload("Groups").Where("id = ? AND disabled = ?", *session.UserID, false).First(&user).Error; err != nil {
		return nil, "", err
	}
	if user.Groups == nil {
		user.Groups = []model.Group{}
	}
	return &user, session.CSRFHash, nil
}
func (s *Service) CheckCSRF(token, csrf string) bool {
	if len(csrf) != 64 {
		return false
	}
	_, expected, err := s.Session(token)
	return err == nil && subtle.ConstantTimeCompare([]byte(hash(csrf)), []byte(expected)) == 1
}
func (s *Service) Login(username, password, previous string) (*model.User, string, string, error) {
	var user model.User
	err := s.S.DB.Preload("Groups").Where("username = ?", username).First(&user).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", "", err
	}
	encoded := s.dummyHash
	if err == nil {
		encoded = []byte(user.PasswordHash)
	}
	matches := bcrypt.CompareHashAndPassword(encoded, []byte(password)) == nil
	if err != nil || user.Disabled || !matches {
		return nil, "", "", problem(401, "用户名或密码错误")
	}
	var token, csrf string
	err = s.S.DB.Transaction(func(tx *gorm.DB) error {
		var current model.User
		if err := tx.First(&current, user.ID).Error; err != nil {
			return err
		}
		if current.Disabled || current.PasswordHash != user.PasswordHash {
			return problem(401, "用户名或密码错误")
		}
		var err error
		token, err = randomToken()
		if err != nil {
			return err
		}
		csrf, err = randomToken()
		if err != nil {
			return err
		}
		if previous != "" {
			if err := tx.Where("token_hash = ?", hash(previous)).Delete(&model.Session{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("expires_at <= ?", time.Now()).Delete(&model.Session{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.Session{TokenHash: hash(token), UserID: &user.ID, CSRFHash: hash(csrf), ExpiresAt: time.Now().Add(7 * 24 * time.Hour)}).Error; err != nil {
			return err
		}
		return audit(tx, user.Username, "auth.login", user.Username)
	})
	if user.Groups == nil {
		user.Groups = []model.Group{}
	}
	return &user, token, csrf, err
}
func (s *Service) Logout(token string) (string, string, error) { return s.Issue(nil, token) }

var validUsername = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)

func passwordHash(password string) (string, error) {
	if len(password) < 12 || len(password) > 72 {
		return "", problem(400, "密码长度须为 12–72 字节")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}
func bounded(value string, max int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= max
}
func audit(tx *gorm.DB, actor, action, target string) error {
	return tx.Create(&model.Audit{Actor: actor, Action: action, Target: target}).Error
}
func administrator(tx *gorm.DB, id uint) (*model.User, error) {
	var user model.User
	if err := tx.Where("id = ? AND disabled = ? AND role = ?", id, false, "admin").First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, problem(403, "需要管理员权限")
		}
		return nil, err
	}
	return &user, nil
}
func loadUser(tx *gorm.DB, id uint) (model.User, error) {
	var user model.User
	err := tx.Preload("Groups").First(&user, id).Error
	if user.Groups == nil {
		user.Groups = []model.Group{}
	}
	return user, dbError(err)
}
func (s *Service) Users() ([]model.User, error) {
	users := []model.User{}
	err := s.S.DB.Preload("Groups").Order("id ASC").Find(&users).Error
	for i := range users {
		if users[i].Groups == nil {
			users[i].Groups = []model.Group{}
		}
	}
	return users, err
}

type UserInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Role        string `json:"role"`
	Disabled    bool   `json:"disabled"`
	GroupIDs    []uint `json:"group_ids"`
}

func validateUser(input *UserInput) error {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !bounded(input.DisplayName, 64) || (input.Role != "admin" && input.Role != "user") {
		return problem(400, "显示名称或角色无效")
	}
	return nil
}
func groupsFor(tx *gorm.DB, ids []uint) ([]model.Group, error) {
	if len(ids) > 100 {
		return nil, problem(400, "用户组过多")
	}
	groups := []model.Group{}
	if len(ids) == 0 {
		return groups, nil
	}
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			return nil, problem(400, "用户组无效或重复")
		}
		seen[id] = true
	}
	if err := tx.Where("id IN ?", ids).Find(&groups).Error; err != nil {
		return nil, err
	}
	if len(groups) != len(ids) {
		return nil, problem(400, "用户组不存在")
	}
	return groups, nil
}
func (s *Service) CreateUser(actorID uint, input UserInput) (model.User, error) {
	var out model.User
	if !validUsername.MatchString(input.Username) {
		return out, problem(400, "用户名须为 3–32 位字母、数字或 _.-")
	}
	if err := validateUser(&input); err != nil {
		return out, err
	}
	encoded, err := passwordHash(input.Password)
	if err != nil {
		return out, err
	}
	err = s.S.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := administrator(tx, actorID)
		if err != nil {
			return err
		}
		groups, err := groupsFor(tx, input.GroupIDs)
		if err != nil {
			return err
		}
		user := model.User{Username: input.Username, DisplayName: input.DisplayName, PasswordHash: encoded, Role: input.Role, Disabled: input.Disabled, Groups: groups}
		if err := tx.Create(&user).Error; err != nil {
			return dbError(err)
		}
		if err := audit(tx, actor.Username, "user.create", user.Username); err != nil {
			return err
		}
		out, err = loadUser(tx, user.ID)
		return err
	})
	return out, err
}
func lastAdmin(tx *gorm.DB, user model.User, newRole string, disabled bool) error {
	if user.Role != "admin" || user.Disabled || (newRole == "admin" && !disabled) {
		return nil
	}
	var count int64
	if err := tx.Model(&model.User{}).Where("role = ? AND disabled = ?", "admin", false).Count(&count).Error; err != nil {
		return err
	}
	if count <= 1 {
		return problem(409, "不能移除、禁用或降级最后一个启用的管理员")
	}
	return nil
}
func (s *Service) UpdateUser(actorID, id uint, input UserInput) (model.User, error) {
	var out model.User
	if err := validateUser(&input); err != nil {
		return out, err
	}
	var encoded string
	var err error
	if input.Password != "" {
		encoded, err = passwordHash(input.Password)
		if err != nil {
			return out, err
		}
	}
	err = s.S.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := administrator(tx, actorID)
		if err != nil {
			return err
		}
		user, err := loadUser(tx, id)
		if err != nil {
			return err
		}
		if err := lastAdmin(tx, user, input.Role, input.Disabled); err != nil {
			return err
		}
		groups, err := groupsFor(tx, input.GroupIDs)
		if err != nil {
			return err
		}
		roleChanged := input.Role != user.Role
		updates := map[string]interface{}{"display_name": input.DisplayName, "role": input.Role, "disabled": input.Disabled}
		if encoded != "" {
			updates["password_hash"] = encoded
		}
		if err := tx.Model(&user).Updates(updates).Error; err != nil {
			return dbError(err)
		}
		if err := tx.Model(&user).Association("Groups").Replace(groups); err != nil {
			return err
		}
		if encoded != "" || input.Disabled || roleChanged {
			if err := tx.Where("user_id = ?", id).Delete(&model.Session{}).Error; err != nil {
				return err
			}
		}
		if err := audit(tx, actor.Username, "user.update", user.Username); err != nil {
			return err
		}
		if encoded != "" {
			if err := audit(tx, actor.Username, "user.password_reset", user.Username); err != nil {
				return err
			}
		}
		out, err = loadUser(tx, id)
		return err
	})
	return out, err
}
func (s *Service) DeleteUser(actorID, id uint) error {
	return s.S.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := administrator(tx, actorID)
		if err != nil {
			return err
		}
		if actorID == id {
			return problem(409, "不能删除当前登录账号")
		}
		user, err := loadUser(tx, id)
		if err != nil {
			return err
		}
		if err := lastAdmin(tx, user, "", true); err != nil {
			return err
		}
		if err := tx.Model(&user).Association("Groups").Clear(); err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&model.Session{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&user).Error; err != nil {
			return err
		}
		return audit(tx, actor.Username, "user.delete", user.Username)
	})
}
func (s *Service) ChangePassword(id uint, current, password string) error {
	encoded, err := passwordHash(password)
	if err != nil {
		return err
	}
	var user model.User
	if err := s.S.DB.First(&user, id).Error; err != nil {
		return dbError(err)
	}
	if user.Disabled || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(current)) != nil {
		return problem(400, "当前密码错误")
	}
	return s.S.DB.Transaction(func(tx *gorm.DB) error {
		var live model.User
		if err := tx.First(&live, id).Error; err != nil {
			return err
		}
		if live.Disabled || live.PasswordHash != user.PasswordHash {
			return problem(409, "账号状态已变化，请重新登录")
		}
		if err := tx.Model(&live).Update("password_hash", encoded).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&model.Session{}).Error; err != nil {
			return err
		}
		return audit(tx, live.Username, "account.password", live.Username)
	})
}
func (s *Service) Audits() ([]model.Audit, error) {
	rows := []model.Audit{}
	err := s.S.DB.Order("id DESC").Limit(200).Find(&rows).Error
	return rows, err
}
