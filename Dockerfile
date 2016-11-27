FROM scratch

ADD keycloak-operator-linux-static /bin/operator

ENTRYPOINT ["/bin/operator"]