package commands

import (
	"bet/core"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

type ListBetsCommand struct {
	Core *core.Core
}

func (c *ListBetsCommand) Command() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "bets",
		Description: "See which bets you've placed",
	}
}

func (c *ListBetsCommand) Interaction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	uid := i.Interaction.Member.User.ID
	rows, err := c.Core.Database.LoadUserBets(uid)
	if err != nil {
		log.Printf("DEBUG: %s requested bets, but error loading from db: %s", uid, err)
		genericError(s, i)
		return
	}
	content := fmt.Sprintf("<@%s> has the following open bets:", uid)
	for rows.Next() {
		var eid string
		var amount int
		var risk float64
		var blob string
		if err := rows.Scan(&eid, &amount, &risk, &blob); err != nil {
			log.Printf("DEBUG: could not scan bet row reading user bets: %s", uid, err)
			genericError(s, i)
			return
		}
		blobInterpret := ""
		event, err := c.Core.GetEvent(eid)
		if err != nil {
			log.Printf("DEBUG: event %s couldn't be loaded", eid, err)
			// Loading the event is just for interpreting the blob.  This isn't
			// a necessary interaction to serve a request.
		} else {
			blobInterpret = event.Interpret(blob)
		}
		content += fmt.Sprintf("\n 1. %d cakes on %s (%s), risk %.2f%%", amount, eid, blobInterpret, risk*100)
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: content,
			AllowedMentions: &discordgo.MessageAllowedMentions{
				// Let's the user be tagged by ID so their name appears without
				// pinging them everytime anyone uses the leaderboard command.
				Parse: []discordgo.AllowedMentionType{},
			},
		},
	})
}
