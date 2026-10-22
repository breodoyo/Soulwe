// Daily affirmation verses for the landing-page hero card. Local data only:
// nothing is fetched and nothing is stored. The verse is picked from the
// visitor's local date, so it is the same all day and changes the next day.
export interface DailyVerse {
  text: string
  reference: string
  translation: 'NIV'
}

export const dailyVerses: DailyVerse[] = [
  {
    text: 'Give thanks to the Lord, for he is good; his love endures forever.',
    reference: 'Psalm 107:1',
    translation: 'NIV',
  },
  {
    text: 'Come to me, all you who are weary and burdened, and I will give you rest.',
    reference: 'Matthew 11:28',
    translation: 'NIV',
  },
  {
    text: 'The Lord is close to the brokenhearted and saves those who are crushed in spirit.',
    reference: 'Psalm 34:18',
    translation: 'NIV',
  },
  {
    text: 'So do not fear, for I am with you; do not be dismayed, for I am your God. I will strengthen you and help you; I will uphold you with my righteous right hand.',
    reference: 'Isaiah 41:10',
    translation: 'NIV',
  },
  {
    text: 'He heals the brokenhearted and binds up their wounds.',
    reference: 'Psalm 147:3',
    translation: 'NIV',
  },
]

// Index of the verse for a given local date: the day of the year modulo the
// number of verses. Calendar-day arithmetic (via UTC from the local Y/M/D)
// keeps it stable across daylight-saving changes. Same all day, +1 each day.
export function dailyVerseIndex(date: Date = new Date()): number {
  const dayOfYear = Math.floor(
    (Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()) -
      Date.UTC(date.getFullYear(), 0, 1)) /
      86_400_000,
  )
  return dayOfYear % dailyVerses.length
}
