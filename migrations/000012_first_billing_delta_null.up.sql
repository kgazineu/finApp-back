-- o delta compara com o registro anterior: o primeiro registro de cada usuário não tem delta
ALTER TABLE billing_registrations ALTER COLUMN delta DROP NOT NULL;

UPDATE billing_registrations r
SET delta = NULL
WHERE r.id = (SELECT min(first.id) FROM billing_registrations first WHERE first.user_id = r.user_id);
