package reminderplugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/iopred/bruxism"
)

// A Reminder holds data about a specific reminder.
type Reminder struct {
	StartTime time.Time
	Time      time.Time
	Requester string
	Target    string
	Message   string
	IsPrivate bool
}

// ReminderPlugin is a plugin that reminds users.
type ReminderPlugin struct {
	sync.RWMutex
	bruxism.SimplePlugin
	bot       *bruxism.Bot
	Reminders []*Reminder
}

var randomTimes = []string{
	"1 minute",
	"10 minutes",
	"1 hour",
	"4 hours",
	"tomorrow",
	"next week",
}

var randomMessages = []string{
	"walk the dog",
	"take pizza out of the oven",
	"check my email",
	"feed the baby",
	"play some quake",
}

func (p *ReminderPlugin) random(list []string) string {
	return list[rand.Intn(len(list))]
}

func (p *ReminderPlugin) randomReminder(service bruxism.Service) string {
	ticks := ""
	if service.Name() == bruxism.DiscordServiceName {
		ticks = "`"
	}

	return fmt.Sprintf("%s%sreminder %s %s%s", ticks, service.CommandPrefix(), p.random(randomTimes), p.random(randomMessages), ticks)
}

func (p *ReminderPlugin) helpFunc(bot *bruxism.Bot, service bruxism.Service, message bruxism.Message, detailed bool) []string {
	help := []string{
		bruxism.CommandHelp(service, "reminder", "<time> <reminder>", "Sets a reminder that is sent after the provided time.")[0],
	}
	if detailed {
		help = append(help, []string{
			"Examples: ",
			p.randomReminder(service),
			p.randomReminder(service),
		}...)
	}
	return help
}

func (p *ReminderPlugin) parseReminder(parts []string) (time.Time, string, error) {
	if parts[0] == "tomorrow" {
		return time.Now().Add(1 * time.Hour * 24), strings.Join(parts[1:], " "), nil
	}

	if parts[0] == "next" {
		switch parts[1] {
		case "week":
			return time.Now().Add(1 * time.Hour * 24 * 7), strings.Join(parts[2:], " "), nil
		case "month":
			return time.Now().Add(1 * time.Hour * 24 * 7 * 4), strings.Join(parts[2:], " "), nil
		case "year":
			return time.Now().Add(1 * time.Hour * 24 * 365), strings.Join(parts[2:], " "), nil
		default:
			return time.Time{}, "", errors.New("Invalid next.")
		}
	}

	i, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, "", err
	}

	switch {
	case strings.HasPrefix(parts[1], "sec"):
		return time.Now().Add(time.Duration(i) * time.Second), strings.Join(parts[2:], " "), nil
	case strings.HasPrefix(parts[1], "min"):
		return time.Now().Add(time.Duration(i) * time.Minute), strings.Join(parts[2:], " "), nil
	case strings.HasPrefix(parts[1], "hour"):
		return time.Now().Add(time.Duration(i) * time.Hour), strings.Join(parts[2:], " "), nil
	case strings.HasPrefix(parts[1], "day"):
		return time.Now().Add(time.Duration(i) * time.Hour * 24), strings.Join(parts[2:], " "), nil
	case strings.HasPrefix(parts[1], "week"):
		return time.Now().Add(time.Duration(i) * time.Hour * 24 * 7), strings.Join(parts[2:], " "), nil
	case strings.HasPrefix(parts[1], "month"):
		return time.Now().Add(time.Duration(i) * time.Hour * 24 * 7 * 4), strings.Join(parts[2:], " "), nil
	case strings.HasPrefix(parts[1], "year"):
		return time.Now().Add(time.Duration(i) * time.Hour * 24 * 365), strings.Join(parts[2:], " "), nil
	}

	return time.Time{}, "", errors.New("Invalid string.")
}

// AddReminder adds a reminder.
func (p *ReminderPlugin) AddReminder(reminder *Reminder) error {
	p.Lock()
	defer p.Unlock()

	i := 0
	for _, r := range p.Reminders {
		if r.Requester == reminder.Requester {
			i++
			if i > 5 {
				return errors.New("You have too many reminders already.")
			}
		}
	}

	i = 0
	for _, r := range p.Reminders {
		if r.Time.After(reminder.Time) {
			break
		}
		i++
	}

	p.Reminders = append(p.Reminders, reminder)
	copy(p.Reminders[i+1:], p.Reminders[i:])
	p.Reminders[i] = reminder

	return nil
}

func (p *ReminderPlugin) messageFunc(bot *bruxism.Bot, service bruxism.Service, message bruxism.Message) {
	if !service.IsMe(message) {
		if bruxism.MatchesCommand(service, "remind", message) || bruxism.MatchesCommand(service, "reminder", message) {
			_, parts := bruxism.ParseCommand(service, message)

			if len(parts) < 2 {
				service.SendMessage(message.Channel(), fmt.Sprintf("Invalid reminder, no time or message. eg: %s", p.randomReminder(service)))
				return
			}

			t, r, err := p.parseReminder(parts)

			now := time.Now()

			if err != nil || t.Before(now) || t.After(now.Add(time.Hour*24*365+time.Hour)) {
				service.SendMessage(message.Channel(), fmt.Sprintf("Invalid time. eg: %s", strings.Join(randomTimes, ", ")))
				return
			}

			if r == "" {
				service.SendMessage(message.Channel(), fmt.Sprintf("Invalid reminder, no message. eg: %s", p.randomReminder(service)))
				return
			}

			requester := message.UserName()
			if service.Name() == bruxism.DiscordServiceName {
				requester = fmt.Sprintf("<@%s>", message.UserID())
			}

			err = p.AddReminder(&Reminder{
				StartTime: now,
				Time:      t,
				Requester: requester,
				Target:    message.Channel(),
				Message:   r,
				IsPrivate: service.IsPrivate(message),
			})
			if err != nil {
				service.SendMessage(message.Channel(), err.Error())
				return
			}

			service.SendMessage(message.Channel(), fmt.Sprintf("Reminder set for %s.", humanize.Time(t)))
		}
	}
}

// SendReminder sends a reminder.
func (p *ReminderPlugin) SendReminder(service bruxism.Service, reminder *Reminder) {
	if reminder.IsPrivate {
		service.SendMessage(reminder.Target, fmt.Sprintf("%s you set a reminder: %s", humanize.Time(reminder.StartTime), reminder.Message))
	} else {
		service.SendMessage(reminder.Target, fmt.Sprintf("%s %s set a reminder: %s", humanize.Time(reminder.StartTime), reminder.Requester, reminder.Message))
	}
}

// Run will block until a reminder needs to be fired and then fire it.
func (p *ReminderPlugin) Run(bot *bruxism.Bot, service bruxism.Service) {
	for {
		p.RLock()

		if len(p.Reminders) > 0 {
			reminder := p.Reminders[0]
			if time.Now().After(reminder.Time) {
				p.SendReminder(service, reminder)
				p.Reminders = p.Reminders[1:]
				p.RUnlock()
				continue
			}
		}

		p.RUnlock()
		time.Sleep(500 * time.Millisecond)
	}
}

// Load will load plugin state from a byte array.
func (p *ReminderPlugin) Load(bot *bruxism.Bot, service bruxism.Service, data []byte) error {
	if data != nil {
		if err := json.Unmarshal(data, p); err != nil {
			log.Println("Error loading data", err)
		}
	}
	go p.Run(bot, service)
	return nil
}

// Save will save plugin state to a byte array.
func (p *ReminderPlugin) Save() ([]byte, error) {
	return json.Marshal(p)
}

// New will create a new Reminder plugin.
func New() bruxism.Plugin {
	p := &ReminderPlugin{
		SimplePlugin: *bruxism.NewSimplePlugin("Reminder"),
		Reminders:    []*Reminder{},
	}
	p.MessageFunc = p.messageFunc
	p.HelpFunc = p.helpFunc
	return p
}
