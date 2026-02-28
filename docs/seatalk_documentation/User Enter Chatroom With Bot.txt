Event: User Enter Chatroom With Bot
User enter chatroom with bot event callback can only be triggered with users in SeaTalk v3.56 or later
Event Description
This event is triggered when a user successfully enters a 1-on-1 chat chatroom with the bot from any entry point within the SeaTalk App.

Note:

No event callback will be sent if the user enters the chatroom while bot is not online.
No event callback will be sent when user returns to SeaTalk from another app/window unless the user actively re-enters the chatroom.

 

Event Parameter
Header

Parameter

Type

Description

Content-Type

string

Request header format

Signature

string

A signature to ensure that the request is sent by SeaTalk

 

Body

Parameter

Type

Description

event_id

string

The ID of the event

event_type

string

The type of the event. It will be "user_enter_chatroom_with_bot"
timestamp

uint64

The time when this event happened

app_id

string

The ID of the app to receive the event notification

event

object

Event-specific information

∟employee_code

string

The employee_code of the user

         ∟seatalk_id

string

The SeaTalk ID of the user enters the chatroom

∟email

string

 

The email of the user enters the chatroom
 

 

Request Body Sample

{
    "event_id":"1234567",
    "event_type":"user_enter_chatroom_with_bot",
    "timestamp":1611220944,
    "app_id":"abcdefghiklmn",
    "event":{
        "seatalk_id":"1239487273",
        "employee_code":"e_12345678",
        "email":"sample@seatalk.biz",
        }
    }
}
