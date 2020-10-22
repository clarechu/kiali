
### 需要更新镜像

harbor.cloud2go.cn/cloudos-dev/manager:RC4.27.1.gree
harbor.cloud2go.cn/cloudos-dev/pipeline-web:RC4.27.1.gree
harbor.cloud2go.cn/cloudos-dev/pipeline-log:RC4.27.1.gree

```yaml
version: '3.0'
services:
  manager:
    image: ${IMAGE_GROUP}/manager:RC4.27.1.gree
    container_name: manager
    ports:
      - 8030:8080
    restart: always
    environment:
      INFRA: ${INFRA_SERVER_HOST}
      APP_PROFILES_ACTIVE: ${APP_PROFILES_ACTIVE}
      SOCKET_URL: ws://pipeline.${DOMAIN}
      ES_URL: ${ES_URL}
      ES_TOKEN: ${ES_TOKEN}
      MYSQL_HOST: ${INFRA_SERVER_HOST}
      MYSQL_PASSWORD: ${MYSQL_PASSWORD}
      #LOG_URL: ws://${INFRA_SERVER_HOST}:8884
      AMQP_HOST: ${INFRA_SERVER_HOST}
      AMQP_USER: ${AMQP_USER}
      AMQP_PASSWORD: ${AMQP_PASSWORD}

docker run -p 8030:8080 -e INFRA=${INFRA_SERVER_HOST} \
      APP_PROFILES_ACTIVE: ${APP_PROFILES_ACTIVE} \
      SOCKET_URL: ws://pipeline.${DOMAIN} \
      ES_URL: ${ES_URL}\
      ES_TOKEN: ${ES_TOKEN}\
      MYSQL_HOST: ${INFRA_SERVER_HOST}\
      MYSQL_PASSWORD: ${MYSQL_PASSWORD}\
      #LOG_URL: ws://${INFRA_SERVER_HOST}:8884\
      AMQP_HOST: ${INFRA_SERVER_HOST}\
      AMQP_USER: ${AMQP_USER}\
      AMQP_PASSWORD: ${AMQP_PASSWORD}\
      ${IMAGE_GROUP}/manager:RC4.27.1.gree

  pipeline-web:
    image: ${IMAGE_GROUP}/pipeline-web:RC4.27.1.gree
    container_name: pipeline-web
    ports:
      - 8032:8080
      - 8033:8081
    restart: always
    command:
      - npm
      - run
      - simple
    environment:
      INFRA: ${INFRA_SERVER_HOST}:8030
      api_pipeline: http://pipeline.${DOMAIN}:8030
      api_biz_blueprint: http://${INFRA_SERVER_HOST}:8003/composer/blueprint
      api_load_factory: http://${INFRA_SERVER_HOST}:8003/composer/components
      api_load_mart: http://${INFRA_SERVER_HOST}:8002/app/list
      api_dict_data: http://admin.${DOMAIN}/api/dict/data/grid
      api_file: http://${INFRA_SERVER_HOST}:8089
      dbhost: ${INFRA_SERVER_HOST}
      dbpass: ${MYSQL_PASSWORD}
      dbuser: root
      token_key: ${JWT_TOKEN_KEY}
      NODE_VERSION: 8.12.0
      YARN_VERSION: 1.9.4
      DEBUG: "main*,utils*"
      DEBUG_COLORS: "on"

docker run -p 8030:8080 -e INFRA: ${INFRA_SERVER_HOST}:8030 \
      api_pipeline: http://pipeline.${DOMAIN}:8030 \
      api_biz_blueprint: http://${INFRA_SERVER_HOST}:8003/composer/ \blueprint \
      api_load_factory: http://${INFRA_SERVER_HOST}:8003/composer/components \
      api_load_mart: http://${INFRA_SERVER_HOST}:8002/app/list \
      api_dict_data: http://admin.${DOMAIN}/api/dict/data/grid \
      api_file: http://${INFRA_SERVER_HOST}:8089 \
      dbhost: ${INFRA_SERVER_HOST} \
      dbpass: ${MYSQL_PASSWORD} \
      dbuser: root \
      token_key: ${JWT_TOKEN_KEY} \
      NODE_VERSION: 8.12.0 \
      YARN_VERSION: 1.9.4 \
      DEBUG: "main*,utils*" \
      DEBUG_COLORS: "on" \
      -d ${IMAGE_GROUP}/pipeline-web:RC4.27.1.gree npm run simple
  pipeline-log:
    image: ${IMAGE_GROUP}/pipeline-log:RC4.27.1.gree
    container_name: pipeline-log
    ports:
      - 8884:8080
    restart: always
    environment:
      ES_URL: ${ES_URL}
      ES_TOKEN: ${ES_TOKEN}
      MONGO_HOST: ${INFRA_SERVER_HOST}
      IS_OCP: "true"
      CONF_PATH: /etc/conf/ocp.conf
      TURTLE_URL: ${INFRA_SERVER_HOST}:16013
      APP_PROFILES_ACTIVE: env
    volumes:
      - /etc/orchor/ocp.conf:/etc/conf/ocp.conf:ro


    docker run -p 8884:8080 -d ${IMAGE_GROUP}/pipeline-log:RC4.27.1.gree -v /etc/orchor/ocp.conf:/etc/conf/ocp.conf:ro -e ES_URL: ${ES_URL} \
      -e ES_TOKEN: ${ES_TOKEN} \
      -e MONGO_HOST: ${INFRA_SERVER_HOST} \
      -e IS_OCP: "true" \
      -e CONF_PATH: /etc/conf/ocp.conf \
      -e TURTLE_URL: ${INFRA_SERVER_HOST}:16013 \
      -e APP_PROFILES_ACTIVE: env ${IMAGE_GROUP}/pipeline-log:RC4.27.1.gree

```

变量的解释

```text
IMAGE_GROUP=harbor.cloud2go.cn/cloudos-dev

INFRA_SERVER_HOST=10.10.13.160
DOMAIN=uat.cloudos.gree

# 服务默认环境变量
APP_PROFILES_ACTIVE=env


随机选择一个openshift集群的es地址与token

ES_URL=https://log.apps.gree-test1.cloudos.gree
ES_TOKEN=eyJhbGciOiJSUzI1NiIsImtpZCI6IldENmw1d2g0Qi0wOTFxYlFsdThxTDI0dVhpcE5VNndyUmVZX1duZlVKdGsifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJvcGVuc2hpZnQtb3BlcmF0b3JzLXJlZGhhdCIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VjcmV0Lm5hbWUiOiJlbGFzdGljc2VhcmNoLW9wZXJhdG9yLXRva2VuLXh6NHJkIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQubmFtZSI6ImVsYXN0aWNzZWFyY2gtb3BlcmF0b3IiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiI0NjQwNTlhYi0xNGVhLTQ4OWItODllMi04ZjdhODUxNDRmYzIiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6b3BlbnNoaWZ0LW9wZXJhdG9ycy1yZWRoYXQ6ZWxhc3RpY3NlYXJjaC1vcGVyYXRvciJ9.IKDpy5RHxfrvO9KphvNKWSTrc3rGTZUWXnykW3vwk0yEVKIa2fy6agBpQ_PYKzpIX1SZnMkwMI1biEnNhdWYTuJeBYqqL6feWS9xVpuipNn2OFQmrjgZD2Rte669B2eGgIKS_GWEgaj0KoZtC-Y-i9T7Uv41f0iz4WPVxViy6ZQ0LGanhhZKSdlQryxdVyKp4kwq6frMyoM0VjPhFbOAIqx6xpAhODi5DrM8REXwNSZnbA7YVIJhzp0XhiWk1PuAY3cO9kOTGNEmEBTR5-FYaJue208M2N5d3id_x48tTEjdc_L7bo0qT0oQfhca0h3LT1VKp-VD7cAqgL5ig2ed_A

# rabbitmq username
AMQP_USER=cloudtogo
# rabbitmq password
AMQP_PASSWORD=foeTtehtgiEApFR

# mysql password
MYSQL_PASSWORD=foeTtehtgiEApFR

# mail smtp协议 username
MAIL_SMTP_USERNAME=pipeline@cloudtogo.cn
# mail smtp协议 password
MAIL_SMTP_PASSWORD=9191!1N1
# mail smtp协议 host
MAIL_SMTP_HOST=smtp.mxhichina.com
# mail smtp协议 port
MAIL_SMTP_PORT=465
# mail pop3协议 username
MAIL_POP3_USERNAME=pipeline@cloudtogo.cn
# mail pop3协议 password
MAIL_POP3_PASSWORD=9191!1N1
# mail pop3协议 host
MAIL_POP3_HOST=pop3.mxhichina.com
# mail pop3协议 port
MAIL_POP3_PORT=110
# 控制前端在headers里面传不传token字段 根据环境改变
JWT_TOKEN_KEY=token
```

