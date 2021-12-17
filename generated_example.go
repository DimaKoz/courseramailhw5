package main

/*
This is the example what will be generated
*/

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

/*

"Functions Of ApiError" section -
there are additional methods for well-known ApiError
to eliminate some doubled code

Beginning of "Functions Of ApiError" section

*/

func NewApiError(text string, httpStatus int) *ApiError {
	return &ApiError{
		HTTPStatus: httpStatus,
		Err:        errors.New(text),
	}
}

func (ae ApiError) PrepApiAnswer() []byte {
	return []byte("{ \"error\":\"" + ae.Error() + "\"}")
}

func (ae ApiError) serve(w http.ResponseWriter) {
	apiData := ae.PrepApiAnswer()
	w.Header().Set("Content-Type", http.DetectContentType(apiData))
	w.WriteHeader(ae.HTTPStatus)
	_, err := w.Write(apiData)
	if err != nil {
		fmt.Println(err.Error())
	}
}

/*
The end of "functions of ApiError" section
*/

/*

"Hardcoded Well-known Errors" section

Beginning of "Hardcoded Well-known Errors" section

*/

var (
	errUnknown      = ApiError{HTTPStatus: http.StatusNotFound, Err: errors.New("unknown method")}
	errBadMethod    = ApiError{HTTPStatus: http.StatusNotAcceptable, Err: errors.New("bad method")}
	errEmptyLogin   = ApiError{HTTPStatus: http.StatusBadRequest, Err: errors.New("login must me not empty")}
	errBadUser      = ApiError{HTTPStatus: http.StatusInternalServerError, Err: errors.New("bad user")}
	errUnauthorized = ApiError{HTTPStatus: http.StatusForbidden, Err: errors.New("unauthorized")}
)


/*

"Hardcoded Well-known Errors" section

The end of "Hardcoded Well-known Errors" section

*/

func serveAnswer(w http.ResponseWriter, v interface{}) {
	data, _ := json.Marshal(v)
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

type RespMyApiProfile struct {
	User       `json:"response"`
	EmptyError string `json:"error"`
}

type RespMyApiCreate struct {
	NewUser    `json:"response"`
	EmptyError string `json:"error"`
}

type RespOtherApiCreate struct {
	OtherUser  `json:"response"`
	EmptyError string `json:"error"`
}

func (srv *MyApi) profile(w http.ResponseWriter, r *http.Request) {

	var login string

	switch r.Method {
	case "GET":
		login = r.URL.Query().Get("login")
		if login == "" {
			errEmptyLogin.serve(w)
			return
		}

	case "POST":
		_ = r.ParseForm()
		login = r.Form.Get("login")
		if login == "" {
			errEmptyLogin.serve(w)
			return
		}
	}

	profileParams := ProfileParams{
		Login: login,
	}
	user, err := srv.Profile(r.Context(), profileParams)

	if err != nil {
		switch err.(type) {
		case ApiError:
			err.(ApiError).serve(w)
			return
		default:
			errBadUser.serve(w)
			return
		}
	}
	resp := RespMyApiProfile{
		User:       *user,
		EmptyError: "",
	}
	serveAnswer(w, resp)
}

func (srv *MyApi) create(w http.ResponseWriter, r *http.Request) {

	if r.Method != "POST" {
		errBadMethod.serve(w)
		return
	}

	if r.Header.Get("X-Auth") != "100500" {
		errUnauthorized.serve(w)
		return
	}

	_ = r.ParseForm()

	login := r.Form.Get("login")
	if login == "" {
		errEmptyLogin.serve(w)
		return
	}

	if len(login) < 10 {
		NewApiError("login len must be >= 10", http.StatusBadRequest).serve(w)
		return
	}

	name := r.Form.Get("name")
	full_name := r.Form.Get("full_name")
	if full_name != "" {
		name = full_name
	}

	status := r.Form.Get("status")
	if status == "" {
		status = "user"
	}
	m := make(map[string]bool)
	m["user"] = true
	m["moderator"] = true
	m["admin"] = true
	_, prs := m[status]
	if prs == false {
		NewApiError("status must be one of [user, moderator, admin]", http.StatusBadRequest).serve(w)
		return
	}

	age, err := strconv.Atoi(r.Form.Get("age"))

	if err != nil {
		NewApiError("age must be int", http.StatusBadRequest).serve(w)
		return
	}

	if age < 0 {
		NewApiError("age must be >= 0", http.StatusBadRequest).serve(w)
		return
	}

	if age > 128 {
		NewApiError("age must be <= 128", http.StatusBadRequest).serve(w)
		return
	}

	createParams := CreateParams{
		Login:  login,
		Name:   name,
		Status: status,
		Age:    age,
	}

	newuser, err := srv.Create(r.Context(), createParams)

	if err != nil {
		switch err.(type) {
		case ApiError:
			err.(ApiError).serve(w)
			return
		default:
			errBadUser.serve(w)
			return
		}
	}
	resp := RespMyApiCreate{
		NewUser:    *newuser,
		EmptyError: "",
	}
	serveAnswer(w, resp)
}

func (srv *MyApi) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	switch r.URL.Path {
	case "/user/profile":
		srv.profile(w, r)
	case "/user/create":
		srv.create(w, r)
	default:
		errUnknown.serve(w)
		return
	}
}

func (srv *OtherApi) create(w http.ResponseWriter, r *http.Request) {

	if r.Method != "POST" {
		errBadMethod.serve(w)
		return
	}

	if r.Header.Get("X-Auth") != "100500" {
		errUnauthorized.serve(w)
		return
	}

	_ = r.ParseForm()

	username := r.Form.Get("username")
	if username == "" {
		errEmptyLogin.serve(w)
		return
	}

	if len(username) < 3 {
		NewApiError("login len must be >= 3", http.StatusBadRequest).serve(w)
		return
	}

	name := r.Form.Get("name")
	account_name := r.Form.Get("account_name")
	if account_name == "" {
		name = strings.ToLower(name)
	} else {
		name = account_name
	}

	class := r.Form.Get("class")
	if class == "" {
		class = "warrior"
	}
	m := make(map[string]bool)
	m["warrior"] = true
	m["sorcerer"] = true
	m["rouge"] = true
	_, prs := m[class]
	if prs == false {
		NewApiError("class must be one of [warrior, sorcerer, rouge]", http.StatusBadRequest).serve(w)
		return
	}

	level, err := strconv.Atoi(r.Form.Get("level"))
	if err != nil {
		NewApiError("level must be int", http.StatusBadRequest).serve(w)
		return
	}
	if !(level >= 1) {
		NewApiError("level must be >= 1", http.StatusBadRequest).serve(w)
		return
	}
	if !(level <= 50) {
		NewApiError("level must be <= 50", http.StatusBadRequest).serve(w)
		return
	}

	otherCreateParams := OtherCreateParams{
		Username: username,
		Name:     name,
		Class:    class,
		Level:    level,
	}

	newuser, err := srv.Create(r.Context(), otherCreateParams)

	if err != nil {
		switch err.(type) {
		case ApiError:
			err.(ApiError).serve(w)
			return
		default:
			errBadUser.serve(w)
			return
		}
	}

	resp := RespOtherApiCreate{
		OtherUser:  *newuser,
		EmptyError: "",
	}
	serveAnswer(w, resp)
}

func (srv *OtherApi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/user/create":
		srv.create(w, r)
	default:
		errUnknown.serve(w)
		return
	}
}
