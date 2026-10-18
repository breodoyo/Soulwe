// Hero mood verses for the landing page check-in. Local data only: nothing
// is sent to a server and nothing is stored. The translation lives here so
// it can be swapped in one place.
export interface MoodVerse {
  mood: string
  text: string
  reference: string
  translation: 'NIV'
}

export const moodVerses: MoodVerse[] = [
  {
    mood: 'Heavy',
    text: 'Come to me, all you who are weary and burdened, and I will give you rest.',
    reference: 'Matthew 11:28',
    translation: 'NIV',
  },
  {
    mood: 'Okay',
    text: 'The Lord is my shepherd, I lack nothing.',
    reference: 'Psalm 23:1',
    translation: 'NIV',
  },
  {
    mood: 'Better',
    text: 'Weeping may stay for the night, but rejoicing comes in the morning.',
    reference: 'Psalm 30:5',
    translation: 'NIV',
  },
  {
    mood: 'At peace',
    text: 'And the peace of God, which transcends all understanding, will guard your hearts and your minds in Christ Jesus.',
    reference: 'Philippians 4:7',
    translation: 'NIV',
  },
  {
    mood: 'Grateful',
    text: 'Give thanks to the Lord, for he is good; his love endures forever.',
    reference: 'Psalm 107:1',
    translation: 'NIV',
  },
]
