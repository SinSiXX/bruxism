package directmessageinviteplugin

import (
	"fmt"
	"strings"

	"github.com/iopred/bruxism"
)

func discordInviteID(id string) string {
	id = strings.Replace(id, "://discordapp.com/invite/", "://discord.gg/", -1)
	id = strings.Replace(id, "https://discord.gg/", "", -1)
	id = strings.Replace(id, "http://discord.gg/", "", -1)
	return id
}

func directMessageInviteMessageFunc(bot *bruxism.Bot, service bruxism.Service, message bruxism.Message) {
	if service.Name() == bruxism.DiscordServiceName && !service.IsMe(message) && service.IsPrivate(message) {
		discord := service.(*bruxism.Discord)

		messageMessage := message.Message()
		id := discordInviteID(messageMessage)
		if id != messageMessage && strings.HasPrefix(messageMessage, "http") {

			if discord.ApplicationClientID != "" {
				service.PrivateMessage(message.UserID(), fmt.Sprintf("Please visit https://discordapp.com/oauth2/authorize?client_id=%s&scope=bot to add Septapus to your server.", discord.ApplicationClientID))
			} else {
				if err := service.Join(id); err != nil {
					if err == bruxism.ErrAlreadyJoined {
						service.PrivateMessage(message.UserID(), "I have already joined that server.")
						return
					}
					fmt.Printf("Error joining %s %v", service.Name(), err)
					return
				}
				service.PrivateMessage(message.UserID(), "I have joined that server.")
			}
		}
	}
}

// NewDirectMessageInvitePlugin creates a new direct message invite plugin.
func NewDirectMessageInvitePlugin() bruxism.Plugin {
	p := bruxism.NewSimplePlugin("DirectMessageInvite")
	p.MessageFunc = directMessageInviteMessageFunc
	return p
}