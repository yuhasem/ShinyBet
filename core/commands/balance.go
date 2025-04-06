// commands is a package for passing commands from chat interfaces to core.
package commands

import (
	"bet/core"
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	balanceReqs = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core_commands_balance_total",
		Help: "Number of times /balance was called",
	})
	balanceSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core_commands_balance_success",
		Help: "Number of times /balance succeeded",
	})
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
	balanceReqs.Inc()
	uid := i.Interaction.Member.User.ID
	slog.Debug("balance interaction started", "user", uid)
	message := "You have"
	options := i.ApplicationCommandData().Options
	if len(options) > 0 {
		uid = options[0].UserValue(s).ID
		message = fmt.Sprintf("<@%s> has", uid)
	}
	user, err := c.Core.GetUser(uid)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s requested balance, could not fetch user: %s", uid, err))
		genericError(s, i)
		return
	}
	balance, inBets, err := user.Balance()
	if err != nil {
		slog.Warn(fmt.Sprintf("%s requested balance, could not load from user object: %s\n", uid, err))
		genericError(s, i)
		return
	}
	row, err := c.Core.Database.Rank(uid)
	var rank int
	for row.Next() {
		if err := row.Scan(&rank); err != nil {
			// Don't return in this case, just degrade to showing balance/bets.
			slog.Warn(fmt.Sprintf("could not load rank for user %s: %v", uid, err))
		}
	}
	content := fmt.Sprintf("%s %d cakes (%d in bets)", message, balance, inBets)
	if rank > 0 {
		content += fmt.Sprintf(", rank %d on leaderboard", rank)
	}
	// Reply with a message like "@User has XXXX cake coins (YY in bets)"
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: content,
		},
	})
	balanceSuccess.Inc()
}
