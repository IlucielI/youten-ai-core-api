-- Seed extended domain templates: PODCAST, LECTURE, and MUSIC_LYRICS
INSERT INTO templates (category_key, name, description, prompt, output_schema, version, is_active)
VALUES
(
    'PODCAST',
    'Podcast & Talkshow',
    'Show notes, guest overview, topic chapters with timestamps, actionable takeaways, golden quotes, and referenced links/books.',
    'Analisis transkrip percakapan podcast atau talkshow berikut ke dalam format show notes terstruktur. Ekstraksi judul episode, profil tamu, ringkasan episode, pembahasan per babak topik (chapters) dengan perkiraan rentang timestamp, poin penting (key takeaways), kutipan berkesan (golden quotes), serta rekomendasi buku, artikel, atau tautan yang disebutkan.',
    $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "episode_title",
    "show_notes",
    "guest_overview",
    "topic_chapters",
    "key_takeaways",
    "golden_quotes"
  ],
  "properties": {
    "episode_title": {
      "type": "string"
    },
    "show_notes": {
      "type": "string"
    },
    "guest_overview": {
      "type": "object",
      "properties": {
        "guest_name": {
          "type": "string"
        },
        "guest_title": {
          "type": "string"
        },
        "bio_or_background": {
          "type": "string"
        }
      }
    },
    "topic_chapters": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "timestamp",
          "topic",
          "summary"
        ],
        "properties": {
          "timestamp": {
            "type": "string"
          },
          "topic": {
            "type": "string"
          },
          "summary": {
            "type": "string"
          }
        }
      }
    },
    "key_takeaways": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "golden_quotes": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "quote",
          "speaker"
        ],
        "properties": {
          "quote": {
            "type": "string"
          },
          "speaker": {
            "type": "string"
          },
          "context": {
            "type": "string"
          }
        }
      }
    },
    "referenced_resources": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "title": {
            "type": "string"
          },
          "type": {
            "type": "string"
          },
          "description": {
            "type": "string"
          }
        }
      }
    }
  }
}$$,
    1,
    TRUE
),
(
    'LECTURE',
    'Lecture, Class & Webinar',
    'Lecture & webinar core summary, key concepts/theories, technical glossary (definition of terms), slide/topic notes, and exam review/discussion questions.',
    'Analisis transkrip perkuliahan, kelas daring, atau webinar akademis/profesional berikut ke dalam ringkasan materi pembelajaran yang komprehensif. Ekstraksi tujuan pembelajaran, ringkasan materi kuliah, konsep dan teori fundamental, glosarium istilah dan definisi teknis, catatan inti per topik bahasan, serta pertanyaan tinjauan ujian atau bahan diskusi mendalam.',
    $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "lecture_title",
    "course_summary",
    "core_concepts",
    "technical_glossary",
    "topic_notes",
    "exam_review_questions"
  ],
  "properties": {
    "lecture_title": {
      "type": "string"
    },
    "course_summary": {
      "type": "string"
    },
    "learning_objectives": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "core_concepts": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "concept",
          "explanation"
        ],
        "properties": {
          "concept": {
            "type": "string"
          },
          "explanation": {
            "type": "string"
          },
          "examples": {
            "type": "string"
          }
        }
      }
    },
    "technical_glossary": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "term",
          "definition"
        ],
        "properties": {
          "term": {
            "type": "string"
          },
          "definition": {
            "type": "string"
          }
        }
      }
    },
    "topic_notes": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "topic",
          "key_points"
        ],
        "properties": {
          "topic": {
            "type": "string"
          },
          "key_points": {
            "type": "array",
            "items": {
              "type": "string"
            }
          }
        }
      }
    },
    "exam_review_questions": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "question",
          "hint_or_key_answer"
        ],
        "properties": {
          "question": {
            "type": "string"
          },
          "hint_or_key_answer": {
            "type": "string"
          }
        }
      }
    }
  }
}$$,
    1,
    TRUE
),
(
    'MUSIC_LYRICS',
    'Music Lyrics & Composition',
    'Lyrics transcription segmented by song structure (Verse, Chorus, Bridge, Outro), emotional tone & mood assessment, and core message/meaning analysis.',
    'Transkripsi dan analisis komposisi lirik musik dari rekaman audio berikut. Susun lirik secara terstruktur berdasarkan bagian lagu (seperti Intro, Verse, Pre-Chorus, Chorus, Bridge, dan Outro). Lakukan analisis nada emosional dan suasana hati (mood), pesan filosofis atau makna inti lagu, serta gaya musik dan instrumen yang teridentifikasi.',
    $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "song_title",
    "structured_lyrics",
    "emotional_tone",
    "core_message"
  ],
  "properties": {
    "song_title": {
      "type": "string"
    },
    "artist_or_performers": {
      "type": "string"
    },
    "musical_style": {
      "type": "string"
    },
    "structured_lyrics": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "section",
          "lyrics"
        ],
        "properties": {
          "section": {
            "type": "string"
          },
          "lyrics": {
            "type": "string"
          }
        }
      }
    },
    "emotional_tone": {
      "type": "object",
      "required": [
        "primary_mood",
        "energy_level"
      ],
      "properties": {
        "primary_mood": {
          "type": "string"
        },
        "energy_level": {
          "type": "string"
        },
        "sentiment_analysis": {
          "type": "string"
        }
      }
    },
    "core_message": {
      "type": "string"
    },
    "thematic_elements": {
      "type": "array",
      "items": {
        "type": "string"
      }
    }
  }
}$$,
    1,
    TRUE
)
ON CONFLICT (category_key) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    prompt = EXCLUDED.prompt,
    output_schema = EXCLUDED.output_schema,
    version = EXCLUDED.version,
    is_active = EXCLUDED.is_active,
    updated_at = CURRENT_TIMESTAMP;
