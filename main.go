package main

import (
	"fmt"
	"log"
	"strings"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"

	"github.com/kr/pretty"
	"net/http"
	"encoding/json"
	"os"
	"sync"
	"time"
	"errors"
	"flag"
)

var dothing *bool = flag.Bool("c", false, "create plist")

func IsDiscWeek(p *spotify.SimplePlaylist) bool {
	return p.Name == "Discover Weekly" && p.Owner.ID == "spotifydiscover"
}

func NewPlistName() string {
	now := time.Now()
	weekday := time.Duration(now.Weekday())
	day := time.Hour * 24
	tostart := -weekday * day
	toend := (6-weekday) * day
	start := now.Add(tostart)
	end := now.Add(toend)
	
	return fmt.Sprintf("Discover Weekly %v/%v/%v - %v/%v/%v", start.Year(), start.Month(), start.Day(), end.Year(), end.Month(), end.Day())
}

func GetTrax(c *spotify.Client, id spotify.ID) ([]spotify.ID, error) {
	discTrax, err := c.GetPlaylistTracksOpt("spotifydiscover", id, nil, "items(track(id, name))")
	if err != nil {
		return nil, errors.New("getplay err: " + err.Error())
	}
	
	ids := make([]spotify.ID, 0, len(discTrax.Tracks))
	for _, t := range discTrax.Tracks {
		fmt.Printf("ID: %v, Name: %v\n", t.Track.ID, t.Track.Name)
		ids = append(ids, spotify.ID(t.Track.ID))
	}
	
	return ids, nil
}

func MakePlist(c *spotify.Client, id spotify.ID) error {
	u, err := c.CurrentUser()
	if err != nil {
		return errors.New("curuser err: " + err.Error())
	}

	uid := u.ID

	pl, err := c.CreatePlaylistForUser(uid, NewPlistName(), false)
	if err != nil {
		return errors.New("create error: " + err.Error())
	}
	
	ids, err := GetTrax(c, id)
	if err != nil {
		return errors.New("gettrax err: " + err.Error())
	}

	snap, err := c.AddTracksToPlaylist(u.ID, pl.ID, ids...)
	if err != nil {
		return errors.New("addtrax err: " + err.Error())
	}
	fmt.Println(snap)

	return nil
}

var done sync.WaitGroup

func main() {
	flag.Parse()
	auth := spotify.NewAuthenticator("http://localhost:8888/", spotify.ScopePlaylistReadPrivate, spotify.ScopePlaylistReadCollaborative)

	authurl := auth.AuthURL("potato")

	fmt.Println(authurl)
	
	coderesult := make(chan *oauth2.Token, 1)

	http.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(204)
		fmt.Println("wrote to req")
		go func() {
			tok, err := auth.Token("potato", req)
			if err != nil {
				log.Fatal(err)
			}
			pretty.Println(tok)
			coderesult <- tok

			go func() {
				done.Add(1)
				defer done.Done()
				if fi, err := os.OpenFile("auth", os.O_WRONLY|os.O_CREATE| os.O_EXCL, 0600); err == nil {
					err := json.NewEncoder(fi).Encode(tok)
					if err != nil {
						log.Println("Error with token writing:", err)
					} else {
						log.Println("Finished writing to file")
					}
					fi.Close()
				} else {
					log.Println("error opening auth file:", err)
				}
			}()
		}()

	})

	go func() {
		err := http.ListenAndServe("localhost:8888", nil)

		if err != nil {
			log.Fatal(err)
		}
	}()
	fmt.Println("running server")
	
	if fi, err := os.Open("auth"); err == nil {
		
		defer fi.Close()
		var mtok oauth2.Token
		err = json.NewDecoder(fi).Decode(&mtok)
		if err != nil {
			log.Println("Bad file for token:", err)
		} else {
			coderesult <- &mtok
		}
	} else {
		log.Println("auth fi err:", err)
	}
	
	tok := <- coderesult	
	
	cli := auth.NewClient(tok)
	
	plays, err := cli.CurrentUsersPlaylists()
	
	if err != nil {
		log.Fatal(err)
	}
	
	news := make(chan spotify.SimplePlaylist, 2)
	disc := make(chan spotify.SimplePlaylist, 1)
	go func() {
		for _, p := range plays.Playlists {
			if strings.Contains(p.Name, "New") {
				news <- p
			} else if IsDiscWeek(&p) {
				disc <- p
			}
		}
		close(news)
		close(disc)
	}()
	
	fmt.Println("news:\n")
	for p := range news {
		pretty.Println(p.Name)
	}
	
	p := <- disc
	pretty.Println(p)
	
	if *dothing {
		err = MakePlist(&cli, p.ID)
		if err != nil {
			log.Println("MakePlist err:", err)
		}
	}
	done.Wait()
}