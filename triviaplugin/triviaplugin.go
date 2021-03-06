package triviaplugin

import (
	"encoding/json"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/iopred/bruxism"
)

type question struct {
	Question string
	Answer   string
}

type questions []*question

func (q questions) Question() *question {
	n := rand.Intn(len(q))
	return q[n]
}

type trivia struct {
	All    questions
	Themes map[string]questions
}

func (t *trivia) Question(theme string) *question {
	q := t.Themes[theme]
	if q == nil {
		q = t.All
	}
	return q.Question()
}

var triviaQuestions *trivia = &trivia{
	All:    questions{},
	Themes: map[string]questions{},
}

type triviaScore struct {
	Name  string
	Score int
}

type triviaChannel struct {
	sync.RWMutex

	Channel    string
	Active     bool
	Theme      string
	Unanswered int

	Answer string
	Hint   string

	Scores map[string]*triviaScore

	hintChan chan bool
}

func (t *triviaChannel) Start(bot *bruxism.Bot, service bruxism.Service, theme string) {
	t.Lock()
	defer t.Unlock()

	if t.Active {
		return
	}

	service.SendMessage(t.Channel, "Trivia started.")

	t.Active = true
	t.Theme = theme
	t.Unanswered = 0

	go t.question(bot, service)

	return
}

func (t *triviaChannel) Stop(bot *bruxism.Bot, service bruxism.Service) {
	t.Lock()
	defer t.Unlock()

	if !t.Active {
		return
	}

	t.Active = false
	if t.hintChan != nil {
		close(t.hintChan)
		t.hintChan = nil
	}

	service.SendMessage(t.Channel, "Trivia stopped.")

	return
}

func (t *triviaChannel) Message(bot *bruxism.Bot, service bruxism.Service, message bruxism.Message) {
	if strings.ToLower(message.Message()) == t.Answer {
		t.Lock()
		defer t.Unlock()

		service.SendMessage(message.Channel(), message.UserName()+" correctly answered the question! ("+t.Answer+")")

		ts := t.Scores[message.UserID()]
		if ts == nil {
			ts = &triviaScore{
				Name: message.UserName(),
			}
			t.Scores[message.UserID()] = ts
		}
		ts.Score++

		t.Unanswered = 0

		if t.hintChan != nil {
			close(t.hintChan)
			t.hintChan = nil
		}
	}
}

func (t *triviaChannel) question(bot *bruxism.Bot, service bruxism.Service) {
	t.Lock()
	question := triviaQuestions.Question(t.Theme)

	hintChan := make(chan bool)
	t.hintChan = hintChan
	t.Answer = strings.ToLower(question.Answer)
	t.Unlock()

	service.SendMessage(t.Channel, question.Question)

	answer := strings.Split(question.Answer, "")
	hint := make([]string, len(answer))

	chars := 0
	for i, s := range answer {
		if s == " " {
			hint[i] = " "
		} else {
			chars++
			hint[i] = "-"
		}
	}

	hints := 3
	if hints > chars {
		hints = chars
	}

	hintTime := (time.Minute * 4) / time.Duration(hints+1)
	hintCount := chars / (hints + 1)

	func() {
		for {
			select {
			case <-hintChan:
				return
			case <-time.After(hintTime):
				if hints == 0 {
					service.SendMessage(t.Channel, "The answer was: "+question.Answer)
					t.Lock()
					t.Unanswered++
					if t.Unanswered > 4 {
						service.SendMessage(t.Channel, "Too many unanswered questions. Trivia stopped.")
						t.Active = false
					}
					t.Unlock()
					return
				} else {
					hints--

					service.SendMessage(t.Channel, "Hint: "+strings.Join(hint, ""))

					for i := 0; i < hintCount; i++ {
						for {
							r := rand.Intn(len(hint))
							if hint[r] == "-" {
								hint[r] = answer[r]
								break
							}
						}
					}
				}
			}
		}
	}()

	t.RLock()
	defer t.RUnlock()
	if t.Active {
		go t.question(bot, service)
	}
}

type triviaPlugin struct {
	sync.RWMutex
	// Map from ChannelID -> triviaChannel
	Channels map[string]*triviaChannel
}

// Name returns the name of the plugin.
func (p *triviaPlugin) Name() string {
	return "Trivia"
}

// Load will load plugin state from a byte array.
func (p *triviaPlugin) Load(bot *bruxism.Bot, service bruxism.Service, data []byte) error {
	if data != nil {
		if err := json.Unmarshal(data, p); err != nil {
			log.Println("Error loading data", err)
		}
	}

	for _, t := range p.Channels {
		if t.Active {
			go t.question(bot, service)
		}
	}

	return nil
}

// Save will save plugin state to a byte array.
func (p *triviaPlugin) Save() ([]byte, error) {
	return json.Marshal(p)
}

// Help returns a list of help strings that are printed when the user requests them.
func (p *triviaPlugin) Help(bot *bruxism.Bot, service bruxism.Service, message bruxism.Message, detailed bool) []string {
	if service.IsPrivate(message) || !(service.IsModerator(message) || service.IsBotOwner(message)) {
		return nil
	}

	return bruxism.CommandHelp(service, "trivia", "<start|stop> [theme]", "Starts or stops trivia with an optional theme.")
}

// Message handler.
func (p *triviaPlugin) Message(bot *bruxism.Bot, service bruxism.Service, message bruxism.Message) {
	defer bruxism.MessageRecover()
	if !service.IsMe(message) && !service.IsPrivate(message) {
		messageChannel := message.Channel()

		if bruxism.MatchesCommand(service, "trivia", message) && (service.IsModerator(message) || service.IsBotOwner(message)) {
			p.Lock()
			tc := p.Channels[messageChannel]
			if tc == nil {
				tc = &triviaChannel{
					Channel: messageChannel,
					Scores:  map[string]*triviaScore{},
				}
				p.Channels[messageChannel] = tc
			}
			p.Unlock()

			_, p := bruxism.ParseCommand(service, message)

			if len(p) == 0 {
				return
			}

			switch p[0] {
			case "start":
				theme := ""
				if len(p) >= 2 {
					theme = p[1]
				}
				tc.Start(bot, service, theme)
			case "stop":
				tc.Stop(bot, service)
			}

		} else {
			p.RLock()
			tc := p.Channels[messageChannel]
			p.RUnlock()
			if tc != nil {
				tc.Message(bot, service, message)
			}
		}
	}
}

// New will create a new slow mode plugin.
func New() bruxism.Plugin {
	return &triviaPlugin{
		Channels: map[string]*triviaChannel{},
	}
}
