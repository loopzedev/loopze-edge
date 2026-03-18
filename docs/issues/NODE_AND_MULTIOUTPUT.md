# Issue: Function Node – Multi-Output Skalierung und Wire-Positionierung fehlerhaft

## Beschreibung

Beim Hinzufügen weiterer Ausgänge am Function Node treten mehrere visuelle Probleme auf:

1. **Ungleichmäßiger Abstand der Outputs**: Neue Ausgänge werden nicht mit konstantem Abstand zueinander positioniert. Der Abstand zwischen den Output-Ports variiert statt gleichmäßig verteilt zu sein.

2. **Node-Höhe skaliert nicht korrekt**: Die Höhe des Nodes passt sich beim Hinzufügen neuer Outputs nicht proportional an. Der Node sollte in der Höhe dynamisch wachsen, sodass alle Outputs mit gleichem Abstand dargestellt werden.

3. **Output-Positionen verschieben sich**: Beim Hinzufügen neuer Outputs verändern sich die Positionen bereits vorhandener Ausgänge, ohne dass bestehende Wire-Verbindungen mitgeführt werden. Bereits verschaltete Wires wandern nicht mit den Ports mit, was zu visuell fehlerhaften Verbindungen führt.

## Erwartetes Verhalten

- Outputs haben immer einen **konstanten, gleichmäßigen Abstand** zueinander.
- Die **Node-Höhe wächst dynamisch** entsprechend der Anzahl der Outputs.
- Beim Hinzufügen/Entfernen von Outputs werden **bestehende Wire-Verbindungen** korrekt an die neuen Port-Positionen angepasst.

## Schritte zur Reproduktion

1. Function Node erstellen
2. Einen oder mehrere zusätzliche Outputs hinzufügen
3. Beobachten: Abstände ungleichmäßig, Node-Höhe passt sich nicht an
4. Wire an einen Output anschließen, dann weiteren Output hinzufügen
5. Beobachten: Wire-Position stimmt nicht mehr mit Port-Position überein
