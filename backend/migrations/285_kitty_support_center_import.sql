-- Replace the Kitty UI/service with upstream support tickets. Keep the old
-- tables intact for recovery; never merge schemas with the same original name.
-- Run before accepting requests, in the migration transaction.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM support_tickets t LEFT JOIN users u ON u.id=t.user_id WHERE u.id IS NULL)
       OR EXISTS (SELECT 1 FROM support_tickets WHERE char_length(subject)>100 OR status NOT IN ('open','answered','closed'))
       OR EXISTS (SELECT 1 FROM support_ticket_replies WHERE char_length(body) NOT BETWEEN 1 AND 5000 OR author_type NOT IN ('user','admin')) THEN
        RAISE EXCEPTION 'Legacy support tickets require a reviewed lossless migration; old tables have been preserved';
    END IF;
END $$;

INSERT INTO support_center_tickets
    (id,user_id,category,title,status,message_count,last_message_at,last_message_role,
     user_unread,admin_unread,closed_at,closed_by_role,created_at,updated_at)
SELECT t.id,t.user_id,'',t.subject,
       CASE t.status WHEN 'open' THEN 'pending' WHEN 'answered' THEN 'replied' ELSE 'closed' END,
       (SELECT count(*) FROM support_ticket_replies r WHERE r.ticket_id=t.id),
       COALESCE(last_reply.created_at,t.updated_at),COALESCE(last_reply.author_type,'user'),
       t.status='answered',t.status='open',t.closed_at,'',t.created_at,t.updated_at
FROM support_tickets t
LEFT JOIN LATERAL (SELECT author_type,created_at FROM support_ticket_replies r WHERE r.ticket_id=t.id ORDER BY id DESC LIMIT 1) last_reply ON true;

INSERT INTO support_center_messages(id,ticket_id,author_role,author_user_id,body,created_at)
SELECT r.id,r.ticket_id,r.author_type,u.id,r.body,r.created_at
FROM support_ticket_replies r LEFT JOIN users u ON u.id=r.author_id;

SELECT setval(pg_get_serial_sequence('support_center_tickets','id'),GREATEST(COALESCE(MAX(id),1),1),MAX(id) IS NOT NULL) FROM support_center_tickets;
SELECT setval(pg_get_serial_sequence('support_center_messages','id'),GREATEST(COALESCE(MAX(id),1),1),MAX(id) IS NOT NULL) FROM support_center_messages;

-- The old module was available without an opt-in switch. Preserve availability,
-- while retaining any explicitly saved administrator preference.
INSERT INTO settings(key,value) VALUES ('support_ticket_enabled','true') ON CONFLICT(key) DO NOTHING;
