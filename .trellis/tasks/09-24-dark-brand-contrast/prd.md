# Dark brand text contrast

The dark primary palette doubles as text and fill colors. Existing text-primary-400
and -300 utilities become dark brown and are unreadable on dark surfaces.

## Scope
- Separate foreground colors through Tailwind textColor, covering existing primary
  and accent text utilities, hover/selection states, opacity and currentColor icons.
- Preserve light-mode colors, brand fills, borders, rings and gradients.
- Test compiled utilities and foreground contrast on actual dark surface tokens.
- Check real presale/auth screens plus representative shared console styles locally.
- Preserve pending public-header work. No production, commit, tag or push actions.
