-- fl_rounds.round_number had no DEFAULT, causing OpenRound INSERT to fail.
-- Create a dedicated sequence so callers can omit round_number on insert.
CREATE SEQUENCE IF NOT EXISTS fl_round_number_seq;
ALTER TABLE fl_rounds
    ALTER COLUMN round_number SET DEFAULT nextval('fl_round_number_seq');
ALTER SEQUENCE fl_round_number_seq OWNED BY fl_rounds.round_number;
