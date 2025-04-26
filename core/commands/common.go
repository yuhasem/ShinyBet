package commands

import (
	"bet/core"
	"bet/core/events"
	"errors"
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"
)

func genericError(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: "Something went wrong processing your request. Please try again later.",
		},
	})
}

func respondToWagerError(s *discordgo.Session, i *discordgo.InteractionCreate, err error) {
	slog.Warn(fmt.Sprintf("error placing wager: %v", err))
	if errors.Is(err, &core.BalanceError{}) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "You don't have enough cakes to make that bet!",
			},
		})
	} else if errors.Is(err, &events.PhaseLengthError{}) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "The predicted phase needs to be greater than the current phase!",
			},
		})
		return
	} else if errors.Is(err, events.BettingClosedError{}) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "Betting on this event is closed.",
			},
		})
	} else {
		genericError(s, i)
	}
}
