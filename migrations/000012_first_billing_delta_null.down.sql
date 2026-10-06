UPDATE billing_registrations SET delta = 0 WHERE delta IS NULL;
ALTER TABLE billing_registrations ALTER COLUMN delta SET NOT NULL;
