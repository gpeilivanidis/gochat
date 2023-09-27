package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
)

const (
	userContextKey ContextKey = "user"
)

type ApiServer struct {
	listenAddr string
	store      Storage
}

func NewApiServer(addr string, store Storage) *ApiServer {
	return &ApiServer{
		listenAddr: addr,
		store:      store,
	}
}

func (s *ApiServer) Run() {
	r := mux.NewRouter()

	// serve frontend
	r.HandleFunc("/", s.handleHomePage)                                   // show login/register, home
	r.HandleFunc("/chat/{chatId}", s.protectMiddleware(s.handleChatPage)) // show chat page

	// api calls
	r.HandleFunc("/api/chats/create", s.protectMiddleware(s.handleCreateChat)) // create chat
	r.HandleFunc("/api/chats/{chatId}", s.protectMiddleware(s.handleChat))     // get/join/leave chat, send/receive messages
	r.HandleFunc("/api/login", s.handleLogin)                                  // login
	r.HandleFunc("/api/register", s.handleRegister)                            // register

	log.Println("server running at port:", s.listenAddr)
	log.Fatal(http.ListenAndServe(s.listenAddr, r))
}

func (s *ApiServer) handleHomePage(w http.ResponseWriter, r *http.Request) {
}

func (s *ApiServer) handleChatPage(w http.ResponseWriter, r *http.Request) {
}

func (s *ApiServer) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.handleGetChat(w, r)
		return
	}
	if r.Method == "POST" {
		s.handleJoinChat(w, r)
		return
	}
	if r.Method == "DELETE" {
		s.handleLeaveChat(w, r)
		return
	} else {
		err := fmt.Errorf("error: method %s not allowed", r.Method)
		http.Error(w, err.Error(), http.StatusMethodNotAllowed)
		return
	}
}

func (s *ApiServer) handleCreateChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		err := fmt.Errorf("error: method %s not allowed", r.Method)
		http.Error(w, err.Error(), http.StatusMethodNotAllowed)
		return
	}
	// get password from front
	createReq := new(CreateChatRequest)
	json.NewDecoder(r.Body).Decode(createReq)

	// hash password
	encPass, err := bcrypt.GenerateFromPassword([]byte(createReq.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
		log.Printf("bcrypt encryption error: %v", err)
		return
	}

	// get user from req context
	user, ok := r.Context().Value(userContextKey).(*User)
	if !ok {
		http.Error(w, "error: not authorized", http.StatusUnauthorized)
		return
	}

	// create chat
	chat, err := s.store.CreateChat(string(encPass), *user)
	if err != nil {
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
		log.Printf("error: chat creation failed: %v", err)
		return
	}

	// update user
	user.Chats = append(user.Chats, chat.Id)
	if err = s.store.UpdateUser(*user); err != nil {
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
		log.Printf("error: user update failed: %v", err)
		return
	}

	// response
	WriteJSON(w, http.StatusCreated, chat.ToJSON())
}

func (s *ApiServer) handleGetChat(w http.ResponseWriter, r *http.Request) {
	// get chat id
	id, err := getChatId(r)
	if err != nil {
		http.Error(w, "error: page not found", http.StatusNotFound)
		return
	}

	// get user from req context
	user, ok := r.Context().Value(userContextKey).(*User)
	if !ok {
		http.Error(w, "error: not authorized", http.StatusUnauthorized)
		return
	}

	// get chat
	chat, err := s.store.GetChatById(id)
	if err != nil {
		http.Error(w, "error: page not found", http.StatusNotFound)
		return
	}

	// check for user in chat
	eq := false
	for _, uid := range user.Chats {
		if uid == id {
			eq = true
			break
		}
	}
	if !eq {
		http.Error(w, "error: page not found", http.StatusNotFound)
		return
	}

	// response
	WriteJSON(w, http.StatusOK, chat.ToJSON())
}

func (s *ApiServer) handleJoinChat(w http.ResponseWriter, r *http.Request) {
	// get join request
	joinReq := new(JoinChatRequest)
	json.NewDecoder(r.Body).Decode(joinReq)

	// get chat
	chat, err := s.store.GetChatById(joinReq.Id)
	if err != nil {
		http.Error(w, "error: page not found", http.StatusNotFound)
		return
	}

	// get user
	user, ok := r.Context().Value(userContextKey).(*User)
	if !ok {
		http.Error(w, "error: not authorized", http.StatusUnauthorized)
		return
	}

	// check for password
	if ok := chat.ValidatePassword(joinReq.Password); !ok {
		http.Error(w, "error: not authorized", http.StatusUnauthorized)
		return
	}

	// check for user in chat
	eq := false
	for _, uid := range user.Chats {
		if uid == joinReq.Id {
			eq = true
			break
		}
	}
	if eq {
		return
	}

	// add user to chat
	chat.Users = append(chat.Users, AuthorJSON{Id: user.Id, Username: user.Username})
	user.Chats = append(user.Chats, chat.Id)
	if err := s.store.UpdateChat(*chat); err != nil {
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
		log.Printf("error: update chat failed: %v", err)
		return
	}
	if err := s.store.UpdateUser(*user); err != nil {
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
		log.Printf("error: update user failed: %v", err)
		return
	}

	// response
	WriteJSON(w, http.StatusOK, chat.ToJSON())
}

func (s *ApiServer) handleLeaveChat(w http.ResponseWriter, r *http.Request) {
	// get chat id
	id, err := getChatId(r)
	if err != nil {
		http.Error(w, "error: page not found", http.StatusNotFound)
		return
	}

	// get chat
	chat, err := s.store.GetChatById(id)
	if err != nil {
		http.Error(w, "error: page not found", http.StatusNotFound)
		return
	}

	// get user from req context
	user, ok := r.Context().Value(userContextKey).(*User)
	if !ok {
		http.Error(w, "error: not authorized", http.StatusUnauthorized)
		return
	}

	// check for user in chat
	eq := false
	for _, uid := range user.Chats {
		if uid == id {
			eq = true
			break
		}
	}
	if !eq {
		http.Error(w, "error: page not found", http.StatusNotFound)
		return
	}

	// delete user from chat
	for i, a := range chat.Users {
		if user.Id == a.Id {
			chat.Users = append(chat.Users[:i], chat.Users[i+1:]...)
			break
		}
	}
	for i, cid := range user.Chats {
		if id == cid {
			user.Chats = append(user.Chats[:i], user.Chats[i+1:]...)
			break
		}
	}
	if err := s.store.UpdateChat(*chat); err != nil {
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
		log.Printf("error: update chat failed: %v", err)
		return
	}
	if err := s.store.UpdateUser(*user); err != nil {
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
		log.Printf("error: update user failed: %v", err)
		return
	}

	// response
	WriteJSON(w, http.StatusOK, "chat deleted")
}

func (s *ApiServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	// check req method
	if r.Method != "POST" {
		err := fmt.Errorf("error: method %s not allowed", r.Method)
		http.Error(w, err.Error(), http.StatusMethodNotAllowed)
		return
	}

	// get req
	login := new(LoginRequest)
	json.NewDecoder(r.Body).Decode(login)

	// check if user exists
	user, err := s.store.GetUserByEmail(login.Email)
	if err != nil {
		http.Error(w, "error: user not found", http.StatusBadRequest)
		return
	}

	// check password
	if ok := user.ValidatePassword(login.Password); !ok {
		http.Error(w, "error: invalid password", http.StatusBadRequest)
		return
	}

	// generate token
	token, err := createJWT(user.Id)
	if err != nil {
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
		log.Printf("jwt error: %v", err)
		return
	}

	chats, err := s.store.GetChats(user.Chats)
	if err != nil {
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
		log.Printf("get chats error: %v", err)
		return
	}

	chatsjs := []ChatJSON{}
	for _, c := range chats {
		chatsjs = append(chatsjs, c.ToJSON())
	}

	// response
	res := UserJSON{Id: user.Id, Username: user.Username, Email: user.Email, Chats: chatsjs, Token: token}
	WriteJSON(w, http.StatusCreated, res)
}

func (s *ApiServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	// check req method
	if r.Method != "POST" {
		err := fmt.Errorf("error: method %s not allowed", r.Method)
		http.Error(w, err.Error(), http.StatusMethodNotAllowed)
		return
	}

	// get req
	reg := new(RegisterRequest)
	json.NewDecoder(r.Body).Decode(reg)

	// check for username and email lengths
	if len(reg.Username) > 20 || len(reg.Email) > 50 {
		http.Error(w, "error: username can't be longer than 20 characters and email can't be longer than 50 characters", http.StatusBadRequest)
		return
	}

	// check if user exists
	_, err := s.store.GetUserByEmail(reg.Email)
	if err == nil {
		http.Error(w, "error: user already exists", http.StatusBadRequest)
		return
	}

	// hash password
	encPass, err := bcrypt.GenerateFromPassword([]byte(reg.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
		log.Printf("error: bcrypt encryption error: %v", err)
		return
	}

	// create user in db
	user, err := s.store.CreateUser(reg.Username, reg.Email, string(encPass))
	if err != nil {
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
		log.Printf("error: create user failed: %v", err)
		return
	}

	// generate token
	token, err := createJWT(user.Id)
	if err != nil {
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
		log.Printf("jwt error: %v", err)
		return
	}

	// response
	res := UserJSON{Id: user.Id, Username: user.Username, Email: user.Email, Chats: []ChatJSON{}, Token: token}
	WriteJSON(w, http.StatusCreated, res)
}

func (s *ApiServer) protectMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// check for http header
		header := r.Header.Get("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer") {
			http.Error(w, "error: not authorized", http.StatusUnauthorized)
			return
		}

		// validate token
		tokenString := strings.Split(header, " ")[1]
		token, err := validateJWT(tokenString)
		if err != nil {
			http.Error(w, "error: not authorized", http.StatusUnauthorized)
			return
		}
		if !token.Valid {
			http.Error(w, "error: not authorized", http.StatusUnauthorized)
			return
		}

		// get userId from token
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, "error: not authorized", http.StatusUnauthorized)
			return
		}
		userId, ok := claims["userId"].(float64)
		if !ok {
			http.Error(w, "error: not authorized", http.StatusUnauthorized)
			return
		}
		user, err := s.store.GetUserById(int(userId))
		if err != nil {
			log.Printf("protect error: getUserById err: %v", err)
			http.Error(w, "error: user not found", http.StatusNotFound)
			return
		}

		// call the next func with user in context
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("error: json encoding failed: %v", err)
		http.Error(w, "error: internal server error", http.StatusInternalServerError)
	}
}

func getChatId(r *http.Request) (int, error) {
	ids := mux.Vars(r)["chatId"]
	id, err := strconv.Atoi(ids)
	if err != nil {
		log.Printf("conversion error: %s is not a number", ids)
		return 0, err
	}
	return id, nil
}

func createJWT(id int) (string, error) {
	claims := &jwt.MapClaims{
		"expiresAt": 15000,
		"userId":    id,
	}

	secret := os.Getenv("JWT_SECRET")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(secret))
}

func validateJWT(tokenString string) (*jwt.Token, error) {
	secret := os.Getenv("JWT_SECRET")

	return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// hmacSampleSecret is a []byte containing your secret, e.g. []byte("my_secret_key")
		return []byte(secret), nil
	})
}
