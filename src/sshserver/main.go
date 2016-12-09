package main

import (
	"os"
	"database/sql"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/go-github/github"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"
)

func fatalIfErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

type Config struct {
	HostKey string `yaml:"HostKey"`

	UserAgent    string `yaml:"UserAgent"`
	GitHubID     string `yaml:"GitHubID"`
	GitHubSecret string `yaml:"GitHubSecret"`

	MySQL string `yaml:"MySQL"`

	Listen string `yaml:"Listen"`
	Debug  string `yaml:"Debug"`
}

func main() {
	log.SetOutput(os.Stdout)
	configText, err := ioutil.ReadFile("config.yml")
	fatalIfErr(err)
	var C Config
	fatalIfErr(yaml.Unmarshal(configText, &C))

	go func() {
		log.Println(http.ListenAndServe(C.Debug, nil))
	}()

	t := &github.UnauthenticatedRateLimitedTransport{
		ClientID:     C.GitHubID,
		ClientSecret: C.GitHubSecret,
	}
	GitHubClient := github.NewClient(t.Client())
	GitHubClient.UserAgent = C.UserAgent

	db, err := sql.Open("mysql", C.MySQL)
	fatalIfErr(err)
	fatalIfErr(db.Ping())
	_, err = db.Exec("SET NAMES UTF8")
	fatalIfErr(err)
	query, err := db.Prepare("SELECT `login` FROM `user` LEFT JOIN `pub_key` ON `user_id` = `user`.`id` WHERE `pub_key` = ? LIMIT 1")
	fatalIfErr(err)

	server := &Server{
		githubClient: GitHubClient,
		sqlQuery:     query,
		sessionInfo:  make(map[string]sessionInfo),
	}
	server.sshConfig = &ssh.ServerConfig{
		KeyboardInteractiveCallback: server.KeyboardInteractiveCallback,
		PublicKeyCallback:           server.PublicKeyCallback,
	}

	private, err := ssh.ParsePrivateKey([]byte(C.HostKey))
	fatalIfErr(err)
	server.sshConfig.AddHostKey(private)

	listener, err := net.Listen("tcp", C.Listen)
	fatalIfErr(err)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Accept failed:", err)
			continue
		}

		go server.Handle(conn)
	}
}
