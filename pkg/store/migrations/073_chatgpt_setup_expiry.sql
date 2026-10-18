CREATE POLICY expiry_cleanup ON chatgpt_setups FOR DELETE USING(expires_at<=now());
