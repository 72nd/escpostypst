// #let PAGE_WIDTH = 79.5mm
#let PAGE_WIDTH = 72mm
#let MARGIN = (
  top: 0mm,
  left: 0mm,
  // right: 7.5mm,
  right: 0mm,
  bottom: 0mm,
)

/// Template for 80mm wide paper for the EPSON TM-T20III thermal printer.
#let tpl(
  height: auto,
  font_size: 8pt,
  narrow_par: false,
  body,
) = {
  set page(width: PAGE_WIDTH, height: height, margin: MARGIN)

  set par(
    spacing: 0.9em,
    leading: 0.4em,
  ) if narrow_par

  show heading.where(level: 2): set text(size: 0.9em)
  show heading.where(level: 2): set block(above: .9em)

  set text(
    size: font_size,
    font: "Arial",
    hyphenate: true,
  )

  show raw: set text(font: "Courier New", size: font_size)

  body
}

// #dt.display("[day].[month].[year]") #h(1fr) #box(baseline: 0.1em, width: 2.4em, height: 2.4em, image("../icon-smooth.png")) #h(1fr) #dt.display("[hour]:[minute]:[second]")
/*
#let header_img = {
  set align(center)
  box(baseline: 0.1em, width: 2.4em, height: 2.4em, image("icon-smooth.png"))
}
*/

#let title(size: 15pt, img: false, inline_img: false, content) = [
  #set par(spacing: 0.3em, leading: 0.2em)
  #set align(center)
  #if img {
    figure(image(width: 2.4em, "icon-smooth.png"))
  }
  #set text(size: size, weight: "bold")
  #if inline_img {
    box(
      baseline: 0.2em,
      image(height: 1em, "icon-smooth.png"),
    )
  }
  #content
]

#let seal() = {
  show figure: set block(below: 1.0mm)
  figure(image(width: 23mm, "siegel-02.png"))
}

#let footer(body) = {
  set align(center)
  set text(size: 8.0pt)
  body
}
