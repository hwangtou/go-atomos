### Advantage

1. In golang practise, it's easy to make a program crash if using goroutine incorrectly. Go-Atomos is based on actor
   model, it uses a mailbox rather than channel. You don't have to create a goroutine, instead, you create atoms to do
   concurrent, to protect you from crash causing by un-recover goroutine.
