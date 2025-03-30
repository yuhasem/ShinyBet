package commands

import (
	"bet/core"
	"errors"
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	donateReqs = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core/commands/doante_total",
		Help: "Number of times /donate was called",
	})
	donateSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core/commands/donate_success",
		Help: "Number of times /donate succeeded",
	})
)

type DonateCommand struct {
	Core *core.Core
}

func (c *DonateCommand) Command() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "donate",
		Description: "Donate cakes to another user",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "amount",
				Description: "How many cakes to give",
				Type:        discordgo.ApplicationCommandOptionInteger,
				Required:    true,
			},
			{
				Name:        "to",
				Description: "User to give cakes to",
				Type:        discordgo.ApplicationCommandOptionUser,
				Required:    true,
			},
		},
	}
}

func (c *DonateCommand) Interaction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	donateReqs.Inc()
	slog.Debug("donate interaction started")
	options := i.ApplicationCommandData().Options
	if options[0].IntValue() < 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "It was worth a shot, wasn't it?",
			},
		})
		donateSuccess.Inc()
		return
	}
	if options[0].IntValue() == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "You must donate a positive number of cakes.",
			},
		})
		return
	}
	amount := int(options[0].IntValue())
	givingUserID := i.Interaction.Member.User.ID
	takingUserID := options[1].UserValue(s).ID
	givingUser, err := c.Core.GetUser(givingUserID)
	if err != nil {
		slog.Warn(fmt.Sprintf("error getting giving user: %v", err))
		genericError(s, i)
		return
	}
	takingUser, err := c.Core.GetUser(takingUserID)
	if err != nil {
		slog.Warn(fmt.Sprintf("error getting taking user: %v", err))
		genericError(s, i)
		return
	}
	tx, err := c.Core.Database.OpenTransaction()
	if err != nil {
		slog.Warn(fmt.Sprintf("error opening transaction: %v", err))
		genericError(s, i)
		return
	}
	err = givingUser.Reserve(tx, amount)
	if errors.Is(err, &core.BalanceError{}) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "You don't have enough cakes to donate that much!",
			},
		})
		return
	} else if err != nil {
		slog.Warn(fmt.Sprintf("error reserving donate amount: %v", err))
		genericError(s, i)
		return
	}
	err = givingUser.Resolve(tx, amount, true)
	if err != nil {
		slog.Warn(fmt.Sprintf("error resolving donate amount: %v", err))
		genericError(s, i)
		return
	}
	err = takingUser.Earn(tx, amount)
	if err != nil {
		slog.Warn(fmt.Sprintf("error earning donate amount: %v", err))
		genericError(s, i)
		return
	}
	err = tx.Commit()
	if err != nil {
		slog.Warn(fmt.Sprintf("error committing transaction: %v", err))
		genericError(s, i)
		return
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("<@%s> donated %d cakes to <@%s>!", givingUserID, amount, takingUserID),
			AllowedMentions: &discordgo.MessageAllowedMentions{
				// Let's the user be tagged by ID so their name appears without
				// pinging them everytime anyone uses the donate command.
				// TODO: consider allowing taking user to be pinged, but this
				// could be a spam vector.
				Parse: []discordgo.AllowedMentionType{},
			},
		},
	})
	donateSuccess.Inc()
}
