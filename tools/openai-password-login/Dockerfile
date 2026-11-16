FROM kitty/vm2api-distribution:upstream-20261114
USER root
COPY . /opt/kitty-oauth-login/
RUN python3 /opt/kitty-oauth-login/enable-direct.py && chmod -R a+rX /opt/kitty-oauth-login
WORKDIR /opt/kitty-oauth-login
ENV NODE_ENV=production KIN_CODEX_LOGIN_RUNTIME=/opt/vm2api/codex-login-runtime KIN_CODEX_LOGIN_PYTHON=/opt/vm2api/codex-login-python/bin/python
USER 65532:65532
HEALTHCHECK NONE
ENTRYPOINT ["node", "/opt/kitty-oauth-login/server.mjs"]
CMD []
