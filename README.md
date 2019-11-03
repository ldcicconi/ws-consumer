# ws-consumer
My attempt at abstracting a thing that I can use in the future with multiple projects.

Imageing you hand a machine:
- a websocket address
- a subscription message
- message handling logic, and 
- error handling logic

and the machine will consumes+handle the given websocket feed as you desire, until you tell it to stop.

Error handling will likely remain limited to middleware/waiting.