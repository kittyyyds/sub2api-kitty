-- Retire only Kitty's manual Cookie integration. OAuth tokens, account identity
-- and the upstream browser integration are intentionally preserved.
UPDATE accounts SET
 extra = extra - 'openai_prism' - 'openai_prism_models',
 credentials = credentials - 'prism_cookie' - 'prism_cookie_encrypted'
   - 'prism_project_id' - 'prism_user_id' - 'prism_base_url'
WHERE extra ?| ARRAY['openai_prism','openai_prism_models']
 OR credentials ?| ARRAY['prism_cookie','prism_cookie_encrypted','prism_project_id','prism_user_id','prism_base_url'];
