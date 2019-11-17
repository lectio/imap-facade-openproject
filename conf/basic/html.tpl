<!doctype html>
<html>
  <head>
		<base href="{{ base }}" target="_blank">
    <title>{{ .Subject }}</title>
  </head>
  <body class="">
<br>
{{ .Date | date "02 January 2006" }} | {{ .ReadingTime }}
{{ .Description "html" }}
<hr>
  </body>
</html>
