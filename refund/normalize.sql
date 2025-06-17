-- Normalizes inBets and balance for all users.  To be used after running refund.
-- Run with something like `sqlite3 prod.db '.read normalize.sql'`

-- First, get the actual open bets for all users
CREATE TEMP TABLE realBets AS
SELECT b.uid, SUM(b.amount) AS bets
FROM bets b
INNER JOIN events e ON b.eid = e.id
WHERE unixepoch(b.placed) > unixepoch(e.lastOpen)
  AND unixepoch(b.placed) > unixepoch(e.lastClose)
GROUP BY uid;

-- Print some summaries which can be used to verify things after running.
SELECT COUNT(*) FROM users;

SELECT r.uid, r.bets, u.inBets, u.balance
FROM realBets r
INNER JOIN users u on r.uid = u.id
WHERE r.bets != u.inBets;

-- Next, update users inBets to be exactly what they actually have in bets.
-- Note that this does for ALL users, not just ones that have been through a
-- refund, so this works to normalize in all cases.
UPDATE users
SET inBets = r.bets
FROM users AS u INNER JOIN realBets AS r ON u.id = r.uid
WHERE u.id = users.id;

-- Finally, if any users have less balance than what they have in bets, top them
-- up.
UPDATE users
SET balance = inBets
WHERE inBets > balance;
