// commands is a package for passing commands from chat interfaces to core.
package commands

import (
	"bet/core"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

type BalanceCommand struct {
	Core *core.Core
}

func (c *BalanceCommand) Command() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "balance",
		Description: "See your current balance",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "user",
				Description: "User's balance to view, leave empty to see your own",
				Type:        discordgo.ApplicationCommandOptionUser,
				Required:    false,
			},
		},
	}
}

func (c *BalanceCommand) Interaction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	uid := i.Interaction.Member.User.ID
	message := "You have"
	options := i.ApplicationCommandData().Options
	if len(options) > 0 {
		uid = options[0].UserValue(s).ID
		message = fmt.Sprintf("<@%s> has", uid)
	}
	user, err := c.Core.GetUser(uid)
	if err != nil {
		log.Printf("DEBUG: %s requested balance, could not fetch user: %s", uid, err)
		genericError(s, i)
		return
	}
	balance, inBets, err := user.Balance()
	if err != nil {
		log.Printf("DEBUG: %s requested balance, could not load from user object: %s\n", uid, err)
		genericError(s, i)
		return
	}
	// Reply with a message like "@User has XXXX cake coins (YY in bets)"
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: fmt.Sprintf("%s %d cakes (%d in bets)", message, balance, inBets),
		},
	})
}
