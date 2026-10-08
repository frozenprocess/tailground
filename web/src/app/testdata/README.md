`shares.json` was captured from the original `share.ts` at commit `38a6cc2`, before
extracting the codec. Each case pins the input, encoded token and decoded state.
The defaults case includes the actual default preset; the custom case includes
Unicode, custom probes/SSH users and non-default view settings.

Do not regenerate these fixtures to accommodate a structural refactor. The
preset-derived compression dictionary and tuple ordering are part of the wire
format. A format change needs a compatibility/migration design and must retain
decoding coverage for these existing tokens.
