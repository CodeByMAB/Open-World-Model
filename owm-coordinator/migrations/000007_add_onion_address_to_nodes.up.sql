-- Add onion_address for environments that applied 000001 before the column existed.
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS onion_address TEXT;
