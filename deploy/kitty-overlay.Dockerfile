ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root
RUN rm -rf /app/resources
COPY --chown=sub2api:sub2api resources /app/resources
COPY --chown=sub2api:sub2api sub2api /app/sub2api
RUN chmod 0755 /app/sub2api
