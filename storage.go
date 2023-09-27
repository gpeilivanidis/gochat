package main

import (
	"database/sql"
	"encoding/json"
	"log"

	"github.com/lib/pq"
)

type Storage interface {
	CreateUser(string, string, string) (*User, error)
	GetUserById(int) (*User, error)
	GetUserByEmail(string) (*User, error)
	GetUsers([]int) ([]User, error)
	GetAuthors([]int) ([]AuthorJSON, error)
	UpdateUser(User) error

	CreateChat(string, User) (*Chat, error)
	GetChatById(int) (*Chat, error)
	GetChats([]int) ([]Chat, error)
	UpdateChat(Chat) error
}

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore() (*PostgresStore, error) {
	connStr := "user=postgres dbname=postgres password=gochat sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		return nil, err
	}
	return &PostgresStore{
		db: db,
	}, nil
}

func (s *PostgresStore) Init() error {
	if err := s.createUserTable(); err != nil {
		return err
	}
	if err := s.createChatTable(); err != nil {
		return err
	}
	return nil
}

func (s *PostgresStore) createUserTable() error {
	query := `create table if not exists users (
		id serial primary key,
		username varchar(20),
		email varchar(50),
		password varchar(64),
		chats integer[]
	)`

	_, err := s.db.Exec(query)
	return err
}

func (s *PostgresStore) createChatTable() error {
	query := `create table if not exists chat (
		id serial primary key,
		password varchar(64),
		messages json,
		users integer[]
	)`

	_, err := s.db.Exec(query)
	return err
}

func (s *PostgresStore) CreateUser(username string, email string, password string) (*User, error) {
	// exec query
	query := `insert into users 
	(username, email, password, chats)
	values ($1, $2, $3, $4)
	returning *`
	row := s.db.QueryRow(query, username, email, password, pq.Array([]int{}))

	user := &User{Chats: []int{}}

	// scan row
	if err := row.Scan(&user.Id, &user.Username, &user.Email, &user.Password, pq.Array(&[]sql.NullInt64{})); err != nil {
		log.Println("createUser")
		return nil, err
	}

	return user, nil
}

func (s *PostgresStore) GetUserById(id int) (*User, error) {
	// exec query
	query := `select * from users where id = $1 limit 1`
	row := s.db.QueryRow(query, id)

	user := &User{Chats: []int{}}

	// scan row
	nullArray := []sql.NullInt64{}
	if err := row.Scan(&user.Id, &user.Username, &user.Email, &user.Password, pq.Array(&nullArray)); err != nil {
		log.Println("getUserById")
		return nil, err
	}

	// decode sql arr
	for _, id := range nullArray {
		if id.Valid {
			user.Chats = append(user.Chats, int(id.Int64))
		}
	}

	return user, nil
}

func (s *PostgresStore) GetUserByEmail(email string) (*User, error) {
	// exec query
	query := `select * from users where email = $1 limit 1`
	row := s.db.QueryRow(query, email)

	user := &User{Chats: []int{}}

	// scan row
	nullArray := []sql.NullInt64{}
	if err := row.Scan(&user.Id, &user.Username, &user.Email, &user.Password, pq.Array(&nullArray)); err != nil {
		log.Println("getUserByEmail")
		return nil, err
	}

	// decode sql array
	for _, id := range nullArray {
		if id.Valid {
			user.Chats = append(user.Chats, int(id.Int64))
		}
	}

	return user, nil
}

func (s *PostgresStore) GetUsers(arr []int) ([]User, error) {
	// exec query
	query := `select * from users where id = any($1)`
	rows, err := s.db.Query(query, pq.Array(arr))
	if err != nil {
		log.Println("getUsers query error")
		return nil, err
	}
	defer rows.Close()

	// iterate rows
	users := []User{}
	for rows.Next() {

		user := User{Chats: []int{}}

		// scan row
		nullArray := []sql.NullInt64{}
		if err := rows.Scan(&user.Id, &user.Username, &user.Email, &user.Password, pq.Array(&nullArray)); err != nil {
			log.Println("getUsers scan error")
			return nil, err
		}

		// decode sql array
		for _, id := range nullArray {
			if id.Valid {
				user.Chats = append(user.Chats, int(id.Int64))
			}
		}

		users = append(users, user)
	}
	if err = rows.Err(); err != nil {
		log.Println("getUsers err error")
		return nil, err
	}
	return users, nil
}

func (s *PostgresStore) GetAuthors(arr []int) ([]AuthorJSON, error) {
	// exec query
	query := `select username from users where id = any($1)`
	rows, err := s.db.Query(query, pq.Array(arr))
	if err != nil {
		log.Println("getAuthors query err")
		return nil, err
	}
	defer rows.Close()

	// iterate rows
	result := []AuthorJSON{}
	i := 0
	for rows.Next() {
		// author id
		author := AuthorJSON{Id: arr[i]}

		// scan author username
		if err := rows.Scan(&author.Username); err != nil {
			log.Println("getAuthors scan err")
			return nil, err
		}

		result = append(result, author)
		i++
	}
	if err = rows.Err(); err != nil {
		log.Println("getAuthors err error")
		return nil, err
	}
	return result, nil
}

func (s *PostgresStore) UpdateUser(updatedUser User) error {
	// exec query
	query := `update users set chats=$1 where id=$2`
	if _, err := s.db.Exec(query, pq.Array(updatedUser.Chats), updatedUser.Id); err != nil {
		log.Println("updateUser error")
		return err
	}
	return nil
}

func (s *PostgresStore) CreateChat(password string, user User) (*Chat, error) {
	// exec query
	query := `insert into chat
	(password, messages, users)
	values ($1, $2, $3)
	returning *`

	m := []MessageJSON{}
	u := []AuthorJSON{{
		Id:       user.Id,
		Username: user.Username,
	}}

	// encode json
	mjs, err := json.Marshal(&m)
	if err != nil {
		log.Println("createChat json error")
		return nil, err
	}

	// exec query
	row := s.db.QueryRow(query, password, mjs, pq.Array([]int{user.Id}))

	chat := &Chat{Messages: m, Users: u}

	// scan row
	if err := row.Scan(&chat.Id, &chat.Password, &mjs, pq.Array(&[]sql.NullInt64{})); err != nil {
		log.Println("createChat error")
		return nil, err
	}

	// return chat
	return chat, nil
}

func (s *PostgresStore) GetChatById(id int) (*Chat, error) {
	// exec query
	query := `select * from chat where id = $1 limit 1`
	row := s.db.QueryRow(query, id)

	// encode json
	m := []MessageJSON{}
	mjs, err := json.Marshal(&m)
	if err != nil {
		log.Println("getChatById json error")
		return nil, err
	}

	chat := &Chat{Messages: m, Users: []AuthorJSON{}}

	// scan row
	nullArray := []sql.NullInt64{}
	if err := row.Scan(&chat.Id, &chat.Password, &mjs, pq.Array(&nullArray)); err != nil {
		log.Println("getChatById scan error")
		return nil, err
	}

	// decode messages
	if err = json.Unmarshal(mjs, &m); err != nil {
		log.Println("getChatById json decode error")
		return nil, err
	}

	// decode sql array
	usersId := []int{}
	for _, id := range nullArray {
		if id.Valid {
			usersId = append(usersId, int(id.Int64))
		}
	}

	// get users
	chat.Users, err = s.GetAuthors(usersId)
	if err != nil {
		log.Println("getChatById authors error")
		return nil, err
	}

	return chat, nil
}

func (s *PostgresStore) GetChats(arr []int) ([]Chat, error) {
	// exec query
	query := `select * from chat where id = any($1)`
	rows, err := s.db.Query(query, pq.Array(arr))
	if err != nil {
		log.Println("getChats error")
		return nil, err
	}
	defer rows.Close()

	// go through rows
	chats := []Chat{}
	for rows.Next() {

		// init messages and users
		m := []MessageJSON{}
		u := []AuthorJSON{}

		chat := Chat{Messages: m, Users: u}

		// encode messages
		mjs, err := json.Marshal(&m)
		if err != nil {
			log.Println("getChats json error")
			return nil, err
		}

		// scan row
		nullArray := []sql.NullInt64{}
		if err := rows.Scan(&chat.Id, &chat.Password, &mjs, pq.Array(&nullArray)); err != nil {
			log.Println("getChats scan error")
			return nil, err
		}

		// decode sql array
		usersId := []int{}
		for _, id := range nullArray {
			if id.Valid {
				usersId = append(usersId, int(id.Int64))
			}
		}

		// get users
		chat.Users, err = s.GetAuthors(usersId)
		if err != nil {
			log.Println("getChats author error")
			return nil, err
		}

		chats = append(chats, chat)
	}

	// return chats
	if err = rows.Err(); err != nil {
		log.Println("getChats rows.err error")
		return nil, err
	}
	return chats, nil
}

func (s *PostgresStore) UpdateChat(updatedChat Chat) error {
	// encode messages
	mjs, err := json.Marshal(&updatedChat.Messages)
	if err != nil {
		log.Println("updateChat json error")
		return err
	}

	// get users ids
	usersId := []int{}
	for _, author := range updatedChat.Users {
		usersId = append(usersId, author.Id)
	}

	// exec query
	query := `update chat set messages=$1, users=$2 where id=$3`
	if _, err = s.db.Exec(query, mjs, pq.Array(usersId), updatedChat.Id); err != nil {
		log.Println("updateChat error")
		return err
	}
	return nil
}
