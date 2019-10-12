module github.com/lectio/imap-facade-openproject

go 1.12

replace github.com/emersion/go-imap => github.com/Neopallium/go-imap v1.0.1

replace github.com/foomo/simplecert => github.com/Neopallium/simplecert v1.6.2

require (
	github.com/ProtonMail/go-imap-id v0.0.0-20190926060100-f94a56b9ecde
	github.com/emersion/go-imap v0.0.0-00010101000000-000000000000
	github.com/emersion/go-imap-idle v0.0.0-20190519112320-2704abd7050e
	github.com/emersion/go-imap-move v0.0.0-20190710073258-6e5a51a5b342
	github.com/emersion/go-imap-specialuse v0.0.0-20161227184202-ba031ced6a62
	github.com/emersion/go-imap-unselect v0.0.0-20171113212723-b985794e5f26
	github.com/emersion/go-message v0.10.7
	github.com/foomo/simplecert v0.0.0-00010101000000-000000000000
	github.com/foomo/tlsconfig v0.0.0-20180418120404-b67861b076c9
	github.com/jordan-wright/email v0.0.0-20190819015918-041e0cec78b0
	github.com/lectio/go-json-hal v0.0.0-20191010161423-58c97d0eb226
	github.com/spf13/cobra v0.0.5
	github.com/spf13/viper v1.4.0
)
