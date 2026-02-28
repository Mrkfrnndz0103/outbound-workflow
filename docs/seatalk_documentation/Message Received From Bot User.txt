Event: Message Received From Bot User
API Updates: Effective from 14 November 2024.
Sending Images, Files and Videos to 1-1 chats with Bots on SeaTalk App: Requires SeaTalk App version 3.50 or above, releasing on 28 November 2024, for full functionality.
Event Description
This event is triggered when a bot user sent a message to the bot in a 1-on-1 chat.

Currently, only plain text, image, file and video type messages are supported in this event. Combined forwarded message type is not supported. 

Images, files and videos sent to bots can be downloaded with the URL starting with https://openapi.seatalk.io/messaging/v2/file/. For more information, please refer to this document. The rate limit for this endpoint is 100 requests/min.

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

Size/Length Limit

event_id

string

The ID of the event

 

event_type

string

The type of the event. It will be "message_from_bot_subscriber" in this case

 

timestamp

uint64

The time when this event happened

 

app_id

string

The ID of the app to receive the event notification

 

event

object

Event-specific information

 

         ∟seatalk_id

string

The SeaTalk ID of the message sender

 

∟employee_code

string

The employee_code of the user

 

∟email

string

- The email of the message sender

- Return empty if the message sender does not belong to the same org as bot.

 

∟message

object

The message received

 

∟message_id

string	
The ID of the message

 

Note: For security reasons, a single message will have different message_ids when accessed by different apps.

 

∟quoted_message_id

string 	
The ID of the quoted message (If the message has quoted any message)

 

Notes:

- Only quoting messages sent within the last 7 days is supported.

- For security reasons, a single message will have different message_ids when accessed by different apps.

 

∟thread_id

string	
The ID of the thread this message belongs to.

 

∟tag

string

The message type. 

 

Allowed tags: "text", "combined_forwarded_chat_history", "image", "file", "video"

 

For more details on suppported message types for bots, refer to this document.

 

 

∟text

object

The text message object

 

∟content

string

The text message content

 

∟image

object

The image object

 

∟content

string

The URL of the image. Requires a valid API token to access.

 

The image message expires in 7 days and cannot be downloaded using the URL subsequently.

Max: 250 MB

∟file

object

The file object

 

∟content

string

The URL of the file. Requires a valid API token to access.

 

The file message expires in 7 days and cannot be downloaded using the URL subsequently

Max: 250 MB

∟filename

string

The file name with extension; files with no extension specified will be sent as unidentified files

Max: 100 characters

∟video

object

The video object

 

∟content

string

The URL of the video. Requires a valid API token to access.

 

The video message expires in 7 days and cannot be downloaded using the URL subsequently.

Max: 250 MB

Request Body Sample

{
    "event_id":"1234567",
    "event_type":"message_from_bot_subscriber",
    "timestamp":1611220944,
    "app_id":"abcdefghiklmn",
    "event":{
        "seatalk_id":"1239487273",
        "employee_code":"e_12345678",
        "email":"sample@seatalk.biz",
        "message":{
            "message_id": "rSwS8xiQOrLSSuXkvqSTlbF3ALBcU9naXQ0ntcisCEVVkeK1S6C9cfmo",
            "quoted_message_id": "rSwS8xiQOrLSSuXkvqSTlbF4ALBcU9naXQ3-h_79R6Bg91yP_9rUe7G4",
            "tag":"text",
            "text":{
                "content":"How can I request for leave?"
            }
        }
    }
}

Copy
Message Object

These are sample outputs for the message object.

//text 
{
    "tag":"text",
    "text":{
        "content":"Hello world!",
    },
    "combined_forwarded_chat_history":null,
    "image":null,
    "video":null,
    "file":null
}

//image 
{
    "tag":"image",
    "text":null,
    "combined_forwarded_chat_history":null,
    "image":{
        "content":"https://openapi.seatalk.io/messaging/v2/file/asjewnJHe7dfjsWK8LksdmsMN90JjsdwekjU1efwefscvLKJ"
    },
    "video":null,
    "file":null
}

//video 
{
    "tag":"video",
    "text":null,
    "combined_forwarded_chat_history":null,
    "image":null,
    "video":{
        "content":"https://openapi.seatalk.io/messaging/v2/file/uieLdasuUWhebwrBksadfjBMSFIUEmwkefjhgjksdJKK8GJSFNsdjk"
    },
    "file":null
}

//file
{
    "tag":"file",
    "text":null,
    "combined_forwarded_chat_history":null,
    "image":null,
    "video":null,
    "file":{
        "content":"https://openapi.seatalk.io/messaging/v2/file/lskdfewnOKNFiewbeBKuKEKQW7JWEfjefnqwesdi8JFNekqlkfwqef",
        "filename": "sample.txt"
    }
}
