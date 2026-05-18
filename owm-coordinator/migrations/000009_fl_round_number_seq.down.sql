ALTER TABLE fl_rounds ALTER COLUMN round_number DROP DEFAULT;
DROP SEQUENCE IF EXISTS fl_round_number_seq;
