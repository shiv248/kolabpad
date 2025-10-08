-- Initial schema: document table
CREATE TABLE IF NOT EXISTS document (
	id TEXT PRIMARY KEY,
	text TEXT NOT NULL,
	language TEXT,
	otp TEXT
);
