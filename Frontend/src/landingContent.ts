// Verified data for the public pages. Nothing here is invented: an empty
// array or empty string means "not published yet", and the UI hides the row
// entirely rather than showing a placeholder.

// TODO(verified Kenyan helpline name + number): add one string per
// organisation, only after the details have been checked with that
// organisation. Format: 'Organisation name: number'. While this is empty,
// the crisis strip shows only the emergency-services sentence.
export const crisisHelplines: string[] = []

// TODO(source): the URL of the citation for the two statistics on the
// landing page (the 90% figure and the under-100 psychiatrists figure).
// While this is empty, the figures show no footnote marker and no
// footnote is rendered.
export const statsSourceUrl: string = ''
