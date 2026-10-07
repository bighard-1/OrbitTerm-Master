package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"orbitterm-server/internal/middleware"
	"orbitterm-server/internal/service"
	"orbitterm-server/internal/utils"
)

type contractAuthService struct {
	service.AuthService
	pair *utils.TokenPair
}

func (s contractAuthService) Login(string, string) (*utils.TokenPair, error) {
	return s.pair, nil
}
func (s contractAuthService) Refresh(string) (*utils.TokenPair, error) {
	return s.pair, nil
}
func (s contractAuthService) ChangePassword(uint, string, string) (*utils.TokenPair, error) {
	return s.pair, nil
}

type contractAdminAuthService struct {
	service.AdminAuthService
	pair *utils.TokenPair
}

func (s contractAdminAuthService) Login(string, string, service.AdminRequestMeta) (*utils.TokenPair, error) {
	return s.pair, nil
}

func TestAuthTokenResponsesAlwaysIncludePasswordChangeFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name string
		body string
		want bool
		call func(*gin.Context, *utils.TokenPair)
	}{
		{"login forced", `{"username":"user@example.com","password":"password"}`, true, func(ctx *gin.Context, pair *utils.TokenPair) {
			NewAuthController(contractAuthService{pair: pair}, nil, nil).Login(ctx)
		}},
		{"login regular", `{"username":"user@example.com","password":"password"}`, false, func(ctx *gin.Context, pair *utils.TokenPair) {
			NewAuthController(contractAuthService{pair: pair}, nil, nil).Login(ctx)
		}},
		{"refresh forced", `{"refresh_token":"refresh"}`, true, func(ctx *gin.Context, pair *utils.TokenPair) {
			NewAuthController(contractAuthService{pair: pair}, nil, nil).Refresh(ctx)
		}},
		{"refresh regular", `{"refresh_token":"refresh"}`, false, func(ctx *gin.Context, pair *utils.TokenPair) {
			NewAuthController(contractAuthService{pair: pair}, nil, nil).Refresh(ctx)
		}},
		{"password changed", `{"current_password":"old","new_password":"new"}`, false, func(ctx *gin.Context, pair *utils.TokenPair) {
			ctx.Set(middleware.ContextUserIDKey, uint(1))
			NewAuthController(contractAuthService{pair: pair}, nil, nil).ChangePassword(ctx)
		}},
		{"admin login forced", `{"username":"admin@example.com","password":"password"}`, true, func(ctx *gin.Context, pair *utils.TokenPair) {
			(&AdminController{adminAuthService: contractAdminAuthService{pair: pair}}).Login(ctx)
		}},
		{"admin login regular", `{"username":"admin@example.com","password":"password"}`, false, func(ctx *gin.Context, pair *utils.TokenPair) {
			(&AdminController{adminAuthService: contractAdminAuthService{pair: pair}}).Login(ctx)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			tc.call(ctx, &utils.TokenPair{AccessToken: "access", RefreshToken: "refresh", MustChangePassword: tc.want})
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var envelope struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			got, present := envelope.Data["must_change_password"].(bool)
			if !present || got != tc.want {
				t.Fatalf("must_change_password presence/value = %v/%v, want true/%v; body=%s", present, got, tc.want, recorder.Body.String())
			}
		})
	}
}
