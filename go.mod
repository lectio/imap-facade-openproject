module github.com/lectio/imap-facade-openproject

go 1.12

replace github.com/emersion/go-imap => github.com/Neopallium/go-imap v1.0.1

require (
	github.com/ProtonMail/go-imap-id v0.0.0-20190926060100-f94a56b9ecde
	github.com/emersion/go-imap v0.0.0-00010101000000-000000000000
	github.com/emersion/go-imap-idle v0.0.0-20190519112320-2704abd7050e
	github.com/emersion/go-imap-move v0.0.0-20190710073258-6e5a51a5b342
	github.com/emersion/go-imap-specialuse v0.0.0-20161227184202-ba031ced6a62
	github.com/emersion/go-imap-unselect v0.0.0-20171113212723-b985794e5f26
	github.com/emersion/go-message v0.10.7
	github.com/jordan-wright/email v0.0.0-20190819015918-041e0cec78b0
	github.com/lectio/go-json-hal v0.0.0-20191009182755-56630a489c28
	github.com/spf13/cobra v0.0.5
	github.com/spf13/viper v1.4.0
	golang.org/x/crypto v0.0.0-20190308221718-c2843e01d9a2
)
