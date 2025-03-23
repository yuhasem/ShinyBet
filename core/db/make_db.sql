DROP TABLE IF EXISTS bets;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS events;
DROP VIEW IF EXISTS leaderboard;

CREATE TABLE events(
	id TEXT PRIMARY KEY,
	lastOpen TEXT,
	lastClose TEXT,
	details BLOB
);

CREATE TABLE users(
	id TEXT PRIMARY KEY,
	balance INT,
	inBets INT
);

CREATE TABLE bets(
	uid TEXT REFERENCES users(id),
	eid TEXT REFERENCES events(id),
	placed TEXT,
	amount INT,
	risk NUM,
	bet BLOB,
	PRIMARY KEY (uid, placed)
);

CREATE VIEW leaderboard(id, balance) AS
SELECT id, balance
FROM users
ORDER BY balance DESC
LIMIT 10;

INSERT OR REPLACE INTO events VALUES('shiny', '2025-03-01 00:00:00.000', '2025-02-28 00:00:00.000', '5000');
INSERT OR REPLACE INTO users VALUES('user1', 1000, 0);
INSERT OR REPLACE INTO users VALUES('user2', 500, 100);
INSERT OR REPLACE INTO bets VALUES('user2', 'shiny', '2025-03-01 01:00:00.000', 100, 0.567, 'true,10000');