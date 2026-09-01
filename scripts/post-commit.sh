#!/usr/bin/env sh
# Spiegelt nach jedem Commit auf die erreichbaren Nahspiegel.
#
# Bewusst leise und bewusst folgenlos: der Hook darf einen Commit niemals
# scheitern lassen. Ein blockierter Commit ist ein Grund, Hooks abzuschalten,
# und dann ist gar nichts mehr gesichert (HF-0106).
#
# Sichtbar wird ein Rückstand nicht hier, sondern in der Repo-Ansicht von
# Mission Control. Installiert mit: make mirror-setup
exec ./scripts/mirror-repo.sh --leise
