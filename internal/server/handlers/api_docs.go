package handlers

import (
	"GoTodo/internal/server/utils"
	"net/http"
)

// APIDocsV1Handler serves the public REST API v1 documentation page.
func APIDocsV1Handler(w http.ResponseWriter, r *http.Request) {
	email, _, permissions, loggedIn := utils.GetSessionUser(r)

	context := map[string]interface{}{
		"LoggedIn":    loggedIn,
		"UserEmail":   email,
		"Permissions": permissions,
		"Title":       "GoTodo - REST API v1",
	}

	utils.RenderTemplate(w, r, "api_v1_docs.html", context)
}
