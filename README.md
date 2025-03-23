# Betting Bot

This is a discord bot designed to host bets around when the next shiny will
happen on 40 Cakes Shiny Hunting Bot stream.

## Set up

1. For development work or building from source,
   [setup a golang environment](https://go.dev/doc/install). Pre-built
   executables will also be available in this repository.

1. Download [sqlite](https://www.sqlite.org/docs.html).

   1. Run `sqlite3 prod.db '.read make_db.sql'`

   1. You may choose a different name than prod.db, or have multiple databases
      for testing.  The database name can be set in .env for changing which
      database to read from.

1. Create a [Discord bot](https://discord.com/developers/applications).
   [This tutorial](https://discord.com/developers/docs/quick-start/getting-started)
   has more detailed instructions if needed.

   1. Save the Token and Application ID for this bot.

   1. Install the bot to your Discord server, by using the Install Link from the
      developer portal.

1. Setup .env file

   1. Put a file named `.env` in the same directory as the `bet.exe` executable.
      It should have a structure like
    
      ```
      token: <discord bot token>
      appid: <application id of discord bot>
      dbname: "file:<name of database file>"
      ```
    
   1. See `environment.go` for additional configuration options.

1. Run `bet.exe`
