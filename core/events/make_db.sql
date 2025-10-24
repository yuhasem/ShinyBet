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

CREATE TABLE crons(
	id TEXT PRIMARY KEY,
	lastRun TEXT	
);

CREATE VIEW leaderboard(id, balance, rank) AS
SELECT id, balance, row_number() OVER()
FROM (
  SELECT id, balance
  FROM users
  ORDER BY balance DESC
);

INSERT OR REPLACE INTO events VALUES('shiny', '2025-03-01 00:00:00', '2025-02-28 00:00:00', '5000');
INSERT OR REPLACE INTO events VALUES('item', '2025-03-01 00:00:00', '2025-03-02 00:00:00', '');
INSERT OR REPLACE INTO users VALUES('user1', 1000, 0);
INSERT OR REPLACE INTO users VALUES('user2', 500, 100);
INSERT OR REPLACE INTO users VALUES('user3', 400, 200);
INSERT OR REPLACE INTO bets VALUES('user2', 'shiny', '2025-03-01 01:00:00.000', 100, 0.567, 'true,10000');
INSERT OR REPLACE INTO bets VALUES('user3', 'shiny', '2025-02-28 00:00:00.000', 500, 0.1, 'false,1');
INSERT OR REPLACE INTO bets VALUES('user3', 'shiny', '2025-03-01 02:00:00.000', 200, 0.4, 'false,10');
INSERT OR REPLACE INTO bets VALUES('user3', 'item', '2025-03-01 12:00:00.000', 100, 0.95, 'true');

INSERT OR REPLACE INTO crons VALUES('test', '2025-03-01 12:00:00.000');
