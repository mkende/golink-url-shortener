# overview

We are creating a website that serves as a url shortener (go-link style). It is written in the Go programming language.

The overall behaviour is the following:
- There is a landing page with a large care link button, a search box, a short list of my recently created links (If I'm logged in), and a short list of the current most popular links.
- There are buttons to get to a complete (paginated, 100 links per page) list of my links
- Another button for a complete list of links

# list of links

All lists of links (search results, my links, all links), are paginated and can be sorted up or down by creation date, last use date, number of use or alphabetically. 

# creating a link

A short link can only be named with ascii alphanumeric characters and “-”,  “_”, and “.” that are allowed too, but not / or #. Names are NOT case sensitive (they are all stored as entered by the user to be displayed in the list pages but we index and sort them only based on the lower case version). 

When creating the link there is a button to create a quick link, that gives a random 6 lower case letters+digits name to the link, otherwise the user can use whatever they want. That quick create button also exists on the home page (the length of the random link is configurable).

Any word that is used as one of our endpoint (/create /edit…) is forbidden as a link name.

# configuration

The website must read a simple.conf configuration file (in toml or in format) allowing to specify the title of the website, some favicon (although a default one should be supported), all authentication and database required options (see below), and all others relevant options.

There is a full configuration file template with documentation of each option (and their default value). The user can copy that template to bootstrap the real configuration.

# Coding style

This is a pure vibe coded project, so the produced code should be high quality, with types, good design patterns and error handling, no catch all or dummy values, etc.
Some good test coverage should be done, without exaggeration either (no test files with thousands of lines of code).

That being said, do ask me questions for any important architecture question before making dimensioning choices.

# Auth

Any users that have access to the server must be able to follow a redirect without authentication. Unless the configuration says otherwise (in that case the server must redirect the user correctly after that have authenticated).

Otherwise two Auth modes must be supported (they can be activated independently in the config).

- Tailscale based: the server reads the tailscale user headers to get name, email, profile pic.

- OIDC based: the server must be able to connect to an OIDC  provider, such as Authelia, with modern default to get the same information (following the best practices)

The owner of a link is stored with the link. In addition, the creator of a link can share it with additional users (identified by their email address). Sharing should work even if we haven't seen that user in our system yet.

# link sharing

On the link edit page, if the user is the owner of the link (or the link is shared with them) then they have options to update the link target.

They also have options to share the link with more users by their email or their group (if available, so if we use OIDC I guess as that’s probably not available in the case of tailscale).

The config has a “default domain” option that is added to email addresses when they don’t have domains, as well as a required domain (to force only emails with a single domain).

If possible, we have auto complete for the sharing with our list of known users and groups (groups work only with OIDC I guess if this is possible).

# database

The server can be configured with a sqlite DB (the default) or with an external postgres DB (and any other type of DB that is easy to support).

# advanced redirection 

The redirection links can be created as “simple” (the default) or “advanced”. For simple links if the user goes to go/linkname/foobar or go/linkname#foobar (assuming that go/ is routed to the go link service) then the user is redirected to the target of the linkname link with /foobar or #foobar appended.

For advanced links, the target can use go-template syntax with all standard function available + regexp based function for “match” (partial match by default unless the regex is anchored), “extract” (to extract a part of the link), and “replace” (and maybe something else too if useful). In addition some variables are available:

- path: the complete suffix after the go link (/foobar or #foobar) in our example
- parts: an array of the suffix split on each / (with the / themselves).
- args: an array of each of the arguments passed after the ? character (if any), split on the & character
- ua: the user agent of the user being redirected
- email: the email of the user being redirected if known.

The link also has a “force authenticated user” check box that can be checked to force auth before the redirection.

# import/export

There are import/export endpoint that can be used to generate a full json dump of the database and to load it. It can be used only by an authenticated user that is listed as administrator in the config or that is a member of the administrator group named in the config (when using OIDC where we can get groups).

# API access

We have an HTTP REST API for import/export/create links/edit link (with field mask to decide which fields are edited)/delete link/resolve link.

The API is authenticated through API keys that can be created by administrator (with a descriptive name) on the /apikeys page. The API are served on the /edit /new /import and /export page, etc., the same as the HTML content, by specifying JSON content type instead of HTML.

# documentation

You will generate a markdown documentation of the API in a docs/api.md folder.

The website itself will serve a /help page with an explanation of the redirection pattern (both simple and advanced). The same content should be in docs/links.md.

There should also be a top level README.md with overview of the capabilities and how to deploy it. With more details in docs/deployment.md (discussing self hosting, hosting on docker, docker compose and kubernetes) and docs/configuration.md with the documentation of the config (that one might be auto-generated with a copy of the configuration template).

# domain redirect

When executing a redirection, the server just serves it directly. For all other use cases (showing the home page, new link page, any of our custom pages, etc.) the server first checks if the user is using the full domain name of the link with https. If that’s not the case (not the full domain name – which is in the config) or not https, the user is first redirected accordingly (keeping the same request). This also happens for links with mandatory auth, when we are using OIDC, we first redirect to the full domain+https, to see if we have the right cookie, before asking for the authentication. With tailscale auth, if we get the auth header, we don’t even do that first redirection.

# CI

There is a CI that runs all our unit tests before accepting any push to the main branch (that is activated only at the end of the development).

There is another CI that builds a docker container for the project that also run for each push to main. It also run when a commit is tagged with vX.Y.Z in that case the container is published it to a registry (ghcr maybe if it’s the easiest?) – make sure to cache the build between the push to main and the tag.

# Security

Perform regular code reviews and fix any possible security issues. We want this to be a production grade software.

# Scalability

The server must be able to scale to 100k different links and thousands of requests per seconds. Make sure that we never have quadratic algorithms (or worse), never attempt to fetch the entire list of links or the entire list of users in a single request, etc.

In particular, makes sure that updating the access count and last access date of the links cannot cause contention.

# License

Add an MIT license to the project (LICENSE.txt), do not add licensing information to each file.

# Other

Feel free to suggest relevant feature or improvement to anything specified here
