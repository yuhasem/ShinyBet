CREATE TABLE IF NOT EXISTS events(
	id TEXT PRIMARY KEY,
	lastOpen TEXT,
	lastClose TEXT,
	details BLOB
);

CREATE TABLE IF NOT EXISTS users(
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
