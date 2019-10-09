echo
  green "Thanking your node_modules for their service..."
  echo
  rm -rf node_modules

  $0 install

  expressions=(
    "Visible mess helps distract us from the true source of the disorder."
    "But when we really delve into the reasons for why we can't let something go, there are only two: an attachment to the past or a fear for the future"
    "It's a very strange phenomenon, but when we reduce what we own and essentially 'detox' our house, it has a detox effect on our bodies as well."
    "The process of facing and selecting our possessions can be quite painful. It forces us to confront our imperfections and inadequacies and the foolish choices we made in the past."
    "A dramatic reorganization of the home causes correspondingly dramatic changes in lifestyle and perspective. It is life transforming."
    "It is not our memories but the person we have become because of those past experiences that we should treasure. This is the lesson these keepsakes teach us when we sort them. The space in which we live should be for the person we are becoming now, not for the person we were in the past."
    "The act of folding is far more than making clothes compact for storage. It is an act of caring, an expression of love and appreciation for the way these clothes support your lifestyle. Therefore, when we fold, we should put our heart into it, thanking our clothes for protecting our bodies."
    "Many people carry this type of negative self-image for years, but it is swept away the instant they experience their own perfectly clean space. This drastic change in self-perception, the belief that you can do anything if you set your mind to it, transforms behavior and lifestyles."
    "By acknowledging their contribution and letting them go with gratitude, you will be able to truly put the things you own, and your life, in order."
    "Can you truthfully say that you treasure something buried so deeply in a closet or drawer that you have forgotten its existence? If things had feelings, they would certainly not be happy. Free them from the prison to which you have relegated them. Help them leave that deserted isle to which you have exiled them."
    "Storage experts are hoarders."
    "The process of assessing how you feel about the things you own, identifying those that have fulfilled their purpose, expressing your gratitude, and bidding them farewell, is really about examining your inner self, a rite of passage to a new life."
    "It is the same with people. Not every person you meet in life will become a close friend or lover. Some you will find hard to get along with or impossible to like. But these people, too, teach you the precious lesson of who you do like, so that you will appreciate those."
    "There are three approaches we can take toward our possessions: face them now, face them sometime, or avoid them until the day we die."
    "To truly cherish the things that are important to you, you must first discard those that have outlived their purpose. To throw away what you no longer need is neither wasteful nor shameful."
    "The question of what you want to own is actually the question of how you want to live your life."
     "Now imagine yourself living in a space that contains only things that spark joy. Isn't this the lifestyle you dream of?"
  )

  index=$( jot -r 1  0 $((${#expressions[@]} - 1)) )

  echo
  green "\"${expressions[index]}\""
  echo "—Marie Kondo"
  echo
