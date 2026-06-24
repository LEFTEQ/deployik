-- Per-project nonce for the STABLE password-bypass link (?_dpkbypass=...).
-- NULL = no link issued yet (lazily created on first read). Rotating the value
-- revokes every previously-minted bypass link for the project.
ALTER TABLE projects ADD COLUMN bypass_nonce TEXT;
