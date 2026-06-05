package main

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
)

//go:embed admin_view.html
var adminViewHTML string

const adminCookieName = "admin_token"

// adminAuth gates the browser-facing pages. Unlike the JSON API (which reads a
// JWT from the Authorization header), the website carries its JWT in an
// HttpOnly cookie set at login. Unauthenticated requests are redirected to the
// login page.
func (api *API) adminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(adminCookieName)
		if err != nil || !api.validAdminToken(cookie.Value) {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func (api *API) validAdminToken(tokenStr string) bool {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.StandardClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.NewValidationError("unexpected signing method", jwt.ValidationErrorSignatureInvalid)
		}
		return api.SigningKey, nil
	})
	return err == nil && token.Valid
}

// handleAdminLogin shows the login form (GET) and authenticates (POST), setting
// the session cookie on success. It reuses the same username/password as the
// JSON API.
func (api *API) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		login := Login{
			Username: r.FormValue("username"),
			Password: r.FormValue("password"),
		}
		if ok, _ := api.Authenticate(&login); !ok {
			api.renderLogin(w, "Username or password incorrect")
			return
		}

		claims := &jwt.StandardClaims{
			Issuer:    api.Username,
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		ss, err := token.SignedString(api.SigningKey)
		if err != nil {
			api.renderLogin(w, "Could not create session, try again")
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     adminCookieName,
			Value:    ss,
			Path:     "/",
			MaxAge:   7 * 24 * 60 * 60,
			HttpOnly: true,
			Secure:   !isDevelopment,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	api.renderLogin(w, "")
}

func (api *API) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

// handleAdminDashboard serves the client-rendered view page (Design 4 / Night
// Console). All data is fetched by the page from /admin/data.
func (api *API) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(adminViewHTML))
}

// itemDTO is the shape the view page consumes. Dates/times are preformatted
// server-side so the browser doesn't have to reason about timezones.
type itemDTO struct {
	ID         uint     `json:"id"`
	Text       string   `json:"text"`
	DayKey     string   `json:"day_key"`
	Date       string   `json:"date"`
	Label      string   `json:"label"`
	Time       string   `json:"time"`
	Classified bool     `json:"classified"`
	Categories []string `json:"categories"`
	Suggested  []string `json:"suggested"`
	Links      []Link   `json:"links"`
}

type dataDTO struct {
	Categories []string  `json:"categories"`
	Items      []itemDTO `json:"items"`
}

// handleAdminData returns the full history and category list as JSON for the
// view page. Items are newest-first; the page groups them by day and derives the
// "needs classifying" worklist from the Classified flag.
func (api *API) handleAdminData(w http.ResponseWriter, r *http.Request) {
	categories, err := api.AllCategoryNames()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, err := api.HistoryItems("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	now := time.Now()
	dtos := make([]itemDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, itemDTO{
			ID:         it.ID,
			Text:       it.Text,
			DayKey:     it.CreatedAt.Format("2006-01-02"),
			Date:       it.CreatedAt.Format("Monday, Jan 2, 2006"),
			Label:      dayLabel(it.CreatedAt, now),
			Time:       it.CreatedAt.Format("3:04 PM"),
			Classified: it.Classified,
			Categories: it.categoryNames(),
			Suggested:  it.suggestionList(),
			Links:      it.linkList(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dataDTO{Categories: categories, Items: dtos})
}

// dayLabel returns "Today"/"Yesterday" for recent items, else "".
func dayLabel(t, now time.Time) string {
	startToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startItem := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	switch {
	case startItem.Equal(startToday):
		return "Today"
	case startItem.Equal(startToday.AddDate(0, 0, -1)):
		return "Yesterday"
	}
	return ""
}

// handleAdminClassify saves the categories chosen for one item (checked
// existing/suggested boxes plus any free-typed names) and marks it classified.
// It is called via fetch from the view page and replies with 204 on success.
func (api *API) handleAdminClassify(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "bad item id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	names := append([]string{}, r.Form["cat"]...)
	for _, raw := range strings.Split(r.FormValue("new"), ",") {
		if n := strings.TrimSpace(raw); n != "" {
			names = append(names, n)
		}
	}

	if err := api.ClassifyItem(uint(id), names); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// renderLogin renders the login page, styled to match the dark view page.
func (api *API) renderLogin(w http.ResponseWriter, errMsg string) {
	errHTML := ""
	if errMsg != "" {
		errHTML = `<p class="error">` + htmlEscape(errMsg) + `</p>`
	}
	page := `<!DOCTYPE html><html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Log in · Printodo</title>
<style>
*{box-sizing:border-box;}
body{margin:0;min-height:100vh;display:grid;place-items:center;
  font-family:"Space Mono",ui-monospace,"SF Mono",monospace;
  background:radial-gradient(80% 60% at 50% -10%,rgba(255,180,84,.12),transparent 60%),#0c0f14;color:#dfe6ef;}
.card{width:320px;max-width:90vw;background:#12161d;border:1px solid #222a35;border-radius:16px;padding:30px 26px;text-align:center;}
.logo{width:42px;height:42px;border-radius:10px;border:1.5px solid #ffb454;color:#ffb454;display:grid;place-items:center;
  font-weight:700;margin:0 auto 14px;box-shadow:0 0 22px -6px #ffb454;font-size:20px;}
h1{font-size:20px;margin:0 0 20px;} h1 span{color:#ffb454;}
input{width:100%;padding:12px;margin-bottom:11px;background:#0c0f14;border:1px solid #222a35;border-radius:9px;
  color:#dfe6ef;font:inherit;font-size:15px;}
input:focus{outline:none;border-color:#ffb454;}
button{width:100%;padding:12px;background:#ffb454;color:#0c0f14;border:none;border-radius:9px;
  font:inherit;font-weight:700;font-size:15px;cursor:pointer;}
button:hover{filter:brightness(1.07);}
.error{color:#ff7a59;font-size:13px;margin:0 0 12px;}
</style></head><body>
<div class="card">
  <div class="logo">P</div>
  <h1>print<span>odo</span></h1>` + errHTML + `
  <form method="POST" action="/admin/login">
    <input type="text" name="username" placeholder="Username" autocapitalize="none" autocomplete="username" required>
    <input type="password" name="password" placeholder="Password" autocomplete="current-password" required>
    <button type="submit">Log in</button>
  </form>
</div></body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page))
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
