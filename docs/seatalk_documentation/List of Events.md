List of Events
This document lists out all the available events for bot users. See Event Callback for how event callback works on SeaTalk Open Platform and how to configure the callback URL for your app.

Category

Event Name

Description

Messaging

 

(Deprecated) new_bot_subscriber

(Deprecated) When a user has subscribed to the bot

user enter chatroom with bot

When a user enters the chatroom with the bot.

message_from_bot_subscriber

When a message is received from a bot user in 1-on-1 chat

new_mentioned_message_from_group_chat

When a group member mentions the bot using '@' in a text message in a group chat. 

interactive_message_click

When a callback button on an interactive message card is clicked by a user

new_message_received_from_thread

when a new message is posted in a thread where the bot has already been previously mentioned or has sent the root message

Group Chat

bot_added_to_group_chat

When a bot user has added the bot to a group chat

bot_removed_from_group_chat

When a bot has been removed from a group chat due to manual removal or group disbandment.