.PHONY: build run clean update

NAME=dripttech-emoji-generator
BUILD=docker build -t $(NAME) .
RUN=docker run  -p 6464:6464 --volume /etc/ssl/elysium:/app/ssl --name $(NAME) --log-opt max-size=10m --restart=always $(ARGS) $(NAME)
RM=docker rm -f $(NAME)
RMI=docker rmi $(NAME)
PRUNE=docker image prune -f

build:
	$(BUILD)
	$(PRUNE)

run:
	$(RUN)

clean:
	$(RM)
	$(RMI)

update:
	$(BUILD)
	$(RM)
	$(RUN)
	$(PRUNE)
