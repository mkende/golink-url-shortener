# TODO

## Bugs

nothing

## Improvements

- store (and display) the time with the creation date.
- display the creation and last used time in the current user timezone.
- have a search box on the home page (no dynamic search), it goes to the all
  links page when you press search (with the right search filter set).

# DONE

## Bugs

nothing

## Improvements

- can we make the go-template execution of the advanced links be much more
  leniant: if any variable is undefined we evaluate it to the empty string but
  the redirection should still work if there is any way for it to succeed.
- The all link page, shoudl have a marker for links that are owned by the
  current user (or shared with him). No need to add that marker to the
  "my links" page. For obvious reasons.
- Allow to sort the all links and my links page by name, target and uses both
  increasing and decreasing.
- Print the total number of links in the title of the /links and my link page
  (e.g. "All Links (293)").
- Print the current Link count of the page (e.g. "200-299 of 438", or
  "1-48 of 48"). This should be below the search text box, just above the table
  headers.
- Make sure that we also have a tag in the link lists to show that a link is
  shared with the current user, not just owned by him. This should also handle
  groups membership, when a link is shared with a group for the current user
  that we have retrieved from OIDC.
- Make sure that the DB queries to gather the list of links, including with the
  owners and sharing are efficient and can scale well.

# VERIFIED

## Bugs

- After creating a link we are redirected to /details/<linkname> and this gives
  a "template not found: details" text error (status 500 in the logs).
- In anonymous mode, when clicking on logout, the /auth/logout page gives a 404
  error. I don't know what happens in other modes but this should be handled
  gracefully (probably redirecting to the homepage).

## Improvements

- Add an "advanced options" button in the "quick create" box on the home page
  that brings us to the "new link" page which shows more options while
  preserving the input already entered by the user (link name and target url).
- In both the home page and the new link page, let's inverse the two text boxes
  to have the target url above the link name.
- Let's not have the logout button when we are logged either through tailscale
  or through the anonymous option (but only when we are logged through OIDC).
  Continue to display the current user email (or the anonymous one).
- When we are logged as anonymous (and only in that case) and we have the OIDC
  provider configured, then we should display the login button to upgrade to an
  OIDC login (not useful in the case of the tailscale provider as that is
  transparent to the user).
- Can we have a built-in /help/advanced page that explains the advanced syntax.
  And there should be a small icon (question mark in a circle) at the end of
  the advanced option in the create link page that leads to that page in a new
  tab (to not loose the user current input).
- The link type in the "create new link" page should be a radio button with
  "Simple" / "Advanced (Go Template)" / "Alias" options, and the target url
  example text (when the box is empty) should be updated accordingly to the
  current option set.
